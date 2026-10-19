package service

import (
	"context"
	"database/sql"
	"fmt"

	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/repository"

	"gorm.io/gorm"
)

// pricingExplicitTransaction keeps the SQLite transaction on one physical connection.
// database/sql marks sql.Tx done before returning a driver COMMIT error, while SQLite can
// leave a deferred-constraint transaction open; issuing SQL directly lets us still rollback.
type pricingExplicitTransaction struct {
	*sql.Conn
	commitContext context.Context
}

func (tx *pricingExplicitTransaction) Commit() error {
	_, err := tx.ExecContext(tx.commitContext, "COMMIT")
	return err
}

func (tx *pricingExplicitTransaction) Rollback() error {
	// Cleanup must still run after the request context is canceled or COMMIT returns its error.
	_, err := tx.ExecContext(context.Background(), "ROLLBACK")
	return err
}

// mutatePricingRevision 串行完成完整价格事务、修订递增和提交后发布。
// 该锁只保护配置/修订的一致读取，不等待已经取得旧 Resolver 的事件批次。
func (s *pricingService) mutatePricingRevision(ctx context.Context, callback func(*gorm.DB) error) (*pricing.Snapshot, int64, error) {
	if s == nil || s.db == nil {
		return nil, 0, fmt.Errorf("database is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	if s.recalculationRunning {
		return nil, 0, ErrPricingBusy
	}

	var candidate *pricing.Snapshot
	var revision int64
	err := s.db.WithContext(ctx).Connection(func(connectionDB *gorm.DB) (err error) {
		connection, ok := connectionDB.Statement.ConnPool.(*sql.Conn)
		if !ok {
			return fmt.Errorf("pricing transaction requires a dedicated database connection")
		}
		if _, err = connection.ExecContext(ctx, "BEGIN"); err != nil {
			return err
		}

		explicitTx := &pricingExplicitTransaction{Conn: connection, commitContext: ctx}
		// TxCommitter prevents dbresolver from switching this session away from the pinned
		// writer connection. Individual GORM writes must not open nested default transactions.
		tx := connectionDB.Session(&gorm.Session{NewDB: true, SkipDefaultTransaction: true})
		tx.Statement.ConnPool = explicitTx
		committed := false
		defer func() {
			if committed {
				return
			}
			if rollbackErr := explicitTx.Rollback(); rollbackErr != nil {
				if err == nil {
					err = fmt.Errorf("rollback pricing transaction: %w", rollbackErr)
				} else {
					err = fmt.Errorf("%w; rollback pricing transaction: %v", err, rollbackErr)
				}
			}
		}()

		if err = callback(tx); err != nil {
			return err
		}
		candidate, err = repository.LoadPricingSnapshot(ctx, tx)
		if err != nil {
			return err
		}
		revision, err = repository.AdvancePricingConfigRevision(tx)
		if err != nil {
			return err
		}
		if err = explicitTx.Commit(); err != nil {
			return err
		}
		committed = true
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	// 只有显式 COMMIT 成功后才发布；失败路径已在同一物理连接上完成 ROLLBACK。
	s.catalog.Replace(candidate)
	return candidate, revision, nil
}

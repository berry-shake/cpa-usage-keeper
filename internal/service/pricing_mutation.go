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

// mutatePricing 串行完成写事务、事务内候选编译和提交后的原子发布。
func (s *pricingService) mutatePricing(ctx context.Context, callback func(*gorm.DB) error) (*pricing.Snapshot, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("database is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()

	var candidate *pricing.Snapshot
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
		if err = explicitTx.Commit(); err != nil {
			return err
		}
		committed = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	// 只有显式 COMMIT 成功后才发布；失败路径已在同一物理连接上完成 ROLLBACK。
	s.catalog.Replace(candidate)
	return candidate, nil
}

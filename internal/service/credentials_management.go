package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

const credentialsStatusWorkerLimit = 10

var ErrCredentialsManagementValidation = errors.New("credentials management request validation failed")

type CredentialsManagementClient interface {
	// UpdateCredentialStatus 返回 CPA 状态码，便于单条开关区分 404 与其它失败。
	UpdateCredentialStatus(ctx context.Context, name string, authIndex string, disabled bool) (int, error)
	DeleteCredentials(ctx context.Context, names []string) error
}

type CredentialsManagementProvider interface {
	SetCredentialsDisabled(ctx context.Context, names []string, disabled bool) (CredentialsManagementResponse, error)
	DeleteCredentials(ctx context.Context, names []string) (CredentialsManagementResponse, error)
}

type CredentialsManagementResponse struct {
	Names    []string `json:"names"`
	Affected int      `json:"affected"`
}

type credentialsManagementService struct {
	client CredentialsManagementClient
	locks  *CredentialMutationLocks
}

func NewCredentialsManagementService(client CredentialsManagementClient, locks *CredentialMutationLocks) CredentialsManagementProvider {
	return &credentialsManagementService{client: client, locks: locks}
}

func (s *credentialsManagementService) SetCredentialsDisabled(ctx context.Context, names []string, disabled bool) (CredentialsManagementResponse, error) {
	cleanNames, err := cleanCredentialNames(names)
	if err != nil {
		return CredentialsManagementResponse{}, err
	}
	if s.client == nil {
		return CredentialsManagementResponse{}, fmt.Errorf("credentials management client is not configured")
	}

	sem := make(chan struct{}, credentialsStatusWorkerLimit)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var updateErr error
	// CPA 的 status 接口一次只接收一个账号；这里在服务端批量 fan-out，并限制并发避免压垮上游。
	for _, name := range cleanNames {
		name := name
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				mu.Lock()
				updateErr = joinCredentialsManagementError(updateErr, ctx.Err())
				mu.Unlock()
				return
			}
			defer s.locks.lockCredential(name)()
			// 批量入口只有文件名，auth_index 留空沿用原有语义。
			if _, err := s.client.UpdateCredentialStatus(ctx, name, "", disabled); err != nil {
				mu.Lock()
				updateErr = joinCredentialsManagementError(updateErr, fmt.Errorf("%s: %w", name, err))
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if updateErr != nil {
		return CredentialsManagementResponse{}, updateErr
	}
	return CredentialsManagementResponse{Names: cleanNames, Affected: len(cleanNames)}, nil
}

func (s *credentialsManagementService) DeleteCredentials(ctx context.Context, names []string) (CredentialsManagementResponse, error) {
	cleanNames, err := cleanCredentialNames(names)
	if err != nil {
		return CredentialsManagementResponse{}, err
	}
	if s.client == nil {
		return CredentialsManagementResponse{}, fmt.Errorf("credentials management client is not configured")
	}
	if err := s.client.DeleteCredentials(ctx, cleanNames); err != nil {
		return CredentialsManagementResponse{}, err
	}
	return CredentialsManagementResponse{Names: cleanNames, Affected: len(cleanNames)}, nil
}

func joinCredentialsManagementError(joined error, err error) error {
	if err == nil {
		return joined
	}
	if joined == nil {
		return err
	}
	if errors.Is(joined, context.Canceled) && errors.Is(err, context.Canceled) {
		return joined
	}
	if errors.Is(joined, context.DeadlineExceeded) && errors.Is(err, context.DeadlineExceeded) {
		return joined
	}
	return errors.Join(joined, err)
}

func cleanCredentialNames(names []string) ([]string, error) {
	seen := make(map[string]struct{}, len(names))
	cleanNames := make([]string, 0, len(names))
	for _, name := range names {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		cleanNames = append(cleanNames, trimmed)
	}
	if len(cleanNames) == 0 {
		return nil, fmt.Errorf("%w: names are required", ErrCredentialsManagementValidation)
	}
	return cleanNames, nil
}

package test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cpa-usage-keeper/internal/cpa"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/repository"
	"cpa-usage-keeper/internal/service"
)

func TestV8MetadataSyncFlattensFirstMatchAndSeparatesConfigAbsenceFromFailure(t *testing.T) {
	db := openMetadataTestDatabase(t, "v8-metadata.db")
	var phase atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v8/management/credentials":
			fmt.Fprint(w, `{"files":[]}`)
		case "/v8/management/config/access/api-keys":
			switch phase.Load() {
			case 0:
				fmt.Fprint(w, `["client-key"]`)
			case 1:
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprint(w, `{"error":"not_found"}`)
			default:
				fmt.Fprint(w, `[]`)
			}
		case "/v8/management/config/api-keys/codex":
			switch phase.Load() {
			case 0:
				fmt.Fprint(w, `[{"name":"unused group name","priority":7,"excluded-models":["*"],"keys":[{"api-key":"first","auth_index":"shared"},{"api-key":"second","auth_index":"second","priority":0,"excluded-models":[]}]},{"priority":99,"keys":[{"api-key":"duplicate","auth_index":"shared"}]}]`)
			case 1:
				w.WriteHeader(http.StatusBadGateway)
				fmt.Fprint(w, `{"error":"unavailable"}`)
			default:
				w.Header().Set("X-CPA-VERSION", "dev")
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprint(w, `{"error":"not_found"}`)
			}
		case "/v8/management/config/api-keys/meta":
			fmt.Fprint(w, `[{"base-url":"  ","keys":[{"api-key":"meta","auth_index":"meta-auth"}]}]`)
		case "/v8/management/config/api-keys/xai", "/v8/management/config/api-keys/gemini", "/v8/management/config/api-keys/interactions", "/v8/management/config/api-keys/claude", "/v8/management/config/api-keys/vertex", "/v8/management/config/api-keys/openai-compatibility":
			w.Header().Set("X-CPA-VERSION", "dev")
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"error":"not_found"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	syncer := service.NewSyncServiceWithClient(db, server.URL, cpa.NewClient(server.URL, "management-secret", time.Second, false))
	if err := syncer.SyncMetadata(context.Background()); err != nil {
		t.Fatal(err)
	}
	identities := loadMetadataIdentityMap(t, db)
	first := identities[metadataIdentityKey(entities.UsageIdentityAuthTypeAIProvider, "shared")]
	second := identities[metadataIdentityKey(entities.UsageIdentityAuthTypeAIProvider, "second")]
	meta := identities[metadataIdentityKey(entities.UsageIdentityAuthTypeAIProvider, "meta-auth")]
	if first.LookupKey != "first" || *first.Priority != 7 || !*first.Disabled || first.Name != "codex" {
		t.Fatalf("first metadata = %+v", first)
	}
	if *second.Priority != 0 || *second.Disabled {
		t.Fatalf("override metadata = %+v", second)
	}
	if meta.BaseURL != "https://api.meta.ai/v1" || *meta.Priority != 0 {
		t.Fatalf("meta metadata = %+v", meta)
	}
	phase.Store(1)
	if err := syncer.SyncMetadata(context.Background()); err == nil {
		t.Fatal("upstream failures returned success")
	}
	first = loadMetadataIdentityMap(t, db)[metadataIdentityKey(entities.UsageIdentityAuthTypeAIProvider, "shared")]
	if first.IsDeleted || first.LookupKey != "first" {
		t.Fatalf("failed provider sync changed row = %+v", first)
	}
	keys, err := repository.ListActiveCPAAPIKeys(db)
	if err != nil || len(keys) != 1 || keys[0].APIKey != "client-key" {
		t.Fatalf("client 404 changed keys = %+v, err = %v", keys, err)
	}
	phase.Store(2)
	if err := syncer.SyncMetadata(context.Background()); err != nil {
		t.Fatal(err)
	}
	first = loadMetadataIdentityMap(t, db)[metadataIdentityKey(entities.UsageIdentityAuthTypeAIProvider, "shared")]
	if !first.IsDeleted {
		t.Fatalf("absent config left active row = %+v", first)
	}
	keys, err = repository.ListActiveCPAAPIKeys(db)
	if err != nil || len(keys) != 0 {
		t.Fatalf("empty access array left keys = %+v, err = %v", keys, err)
	}
}

func TestV8MetadataSyncRequiresCPAHeaderBeforeDeletingAbsentProviderIdentities(t *testing.T) {
	cases := []struct {
		name        string
		status      int
		version     string
		body        string
		wantDeleted bool
	}{
		{"gateway not_found", http.StatusNotFound, "", `{"error":"not_found"}`, false},
		{"blank CPA header", http.StatusNotFound, " \t", `{"error":"not_found"}`, false},
		{"CPA default dev", http.StatusNotFound, "dev", `{"error":"not_found"}`, true},
		{"empty array", http.StatusOK, "", `[]`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := openMetadataTestDatabase(t, "provider-absence.db")
			var refresh atomic.Bool
			var providerCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/v8/management/credentials":
					fmt.Fprint(w, `{"files":[]}`)
				case "/v8/management/config/access/api-keys", "/v8/management/config/api-keys/xai", "/v8/management/config/api-keys/gemini", "/v8/management/config/api-keys/interactions", "/v8/management/config/api-keys/claude", "/v8/management/config/api-keys/vertex", "/v8/management/config/api-keys/meta":
					fmt.Fprint(w, `[]`)
				case "/v8/management/config/api-keys/codex", "/v8/management/config/api-keys/openai-compatibility":
					providerCalls.Add(1)
					if !refresh.Load() {
						authIndex := "codex-auth"
						if r.URL.Path == "/v8/management/config/api-keys/openai-compatibility" {
							authIndex = "openai-auth"
						}
						fmt.Fprintf(w, `[{"name":"provider","keys":[{"api-key":"seed-key","auth_index":%q}]}]`, authIndex)
						return
					}
					if tc.version != "" {
						w.Header().Set("X-CPA-VERSION", tc.version)
					}
					w.WriteHeader(tc.status)
					fmt.Fprint(w, tc.body)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			// 先经真实客户端写入两种来源，再观察缺省判定是否影响 SQLite 中的身份。
			syncer := service.NewSyncServiceWithClient(db, server.URL, cpa.NewClient(server.URL, "management-secret", time.Second, false))
			if err := syncer.SyncMetadata(context.Background()); err != nil {
				t.Fatal(err)
			}
			refresh.Store(true)
			if err := syncer.SyncMetadata(context.Background()); (err != nil) == tc.wantDeleted {
				t.Errorf("refresh error = %v, want deleted = %v", err, tc.wantDeleted)
			}
			identities := loadMetadataIdentityMap(t, db)
			for _, authIndex := range []string{"codex-auth", "openai-auth"} {
				row, ok := identities[metadataIdentityKey(entities.UsageIdentityAuthTypeAIProvider, authIndex)]
				if !ok || row.IsDeleted != tc.wantDeleted || row.LookupKey != "seed-key" {
					t.Errorf("%s identity deleted = %v, found = %v, want deleted = %v", authIndex, row.IsDeleted, ok, tc.wantDeleted)
				}
			}
			if calls := providerCalls.Load(); calls != 4 {
				t.Fatalf("provider requests = %d, want 4", calls)
			}
		})
	}
}

func TestV8MetadataSyncPersistsReturnedNumericTextAndClientKeys(t *testing.T) {
	db := openMetadataTestDatabase(t, "v8-numeric-text.db")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v8/management/credentials":
			fmt.Fprint(w, `{"files":[]}`)
		case "/v8/management/config/access/api-keys":
			fmt.Fprint(w, `[9007199254740993,"00123"]`)
		case "/v8/management/config/api-keys/codex":
			fmt.Fprint(w, `[{"name":123,"prefix":123,"excluded-models":[42],"keys":[{"api-key":9007199254740993,"auth_index":"numeric-native","prefix":null,"excluded-models":null}]}]`)
		case "/v8/management/config/api-keys/openai-compatibility":
			fmt.Fprint(w, `[{"name":9007199254740993,"prefix":789,"keys":[{"api-key":9007199254740993,"auth_index":"numeric-openai"}]}]`)
		case "/v8/management/config/api-keys/xai", "/v8/management/config/api-keys/gemini", "/v8/management/config/api-keys/interactions", "/v8/management/config/api-keys/claude", "/v8/management/config/api-keys/vertex", "/v8/management/config/api-keys/meta":
			w.Header().Set("X-CPA-VERSION", "dev")
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"error":"not_found"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	syncer := service.NewSyncServiceWithClient(db, server.URL, cpa.NewClient(server.URL, "management-secret", time.Second, false))
	if err := syncer.SyncMetadata(context.Background()); err != nil {
		t.Fatal(err)
	}
	identities := loadMetadataIdentityMap(t, db)
	native := identities[metadataIdentityKey(entities.UsageIdentityAuthTypeAIProvider, "numeric-native")]
	openAI := identities[metadataIdentityKey(entities.UsageIdentityAuthTypeAIProvider, "numeric-openai")]
	if native.LookupKey != "9007199254740993" || native.Prefix != "123" || native.Name != "codex" {
		t.Fatalf("native row = %+v", native)
	}
	if openAI.LookupKey != "9007199254740993" || openAI.Prefix != "789" || openAI.Name != "9007199254740993" {
		t.Fatalf("OpenAI row = %+v", openAI)
	}
	keys, err := repository.ListActiveCPAAPIKeys(db)
	if err != nil || len(keys) != 2 {
		t.Fatalf("client keys = %#v, err = %v", keys, err)
	}
	seen := map[string]bool{}
	for _, key := range keys {
		seen[key.APIKey] = true
	}
	if !seen["9007199254740993"] || !seen["00123"] {
		t.Fatalf("client key text changed: %#v", keys)
	}
}

func TestV8MetadataSyncPreservesWholeSourceWhenKeyHasNoAuthIndex(t *testing.T) {
	for _, tc := range []struct {
		name      string
		authField string
	}{
		{"missing", ""},
		{"null", `,"auth_index":null`},
		{"empty", `,"auth_index":""`},
		{"blank", `,"auth_index":" \t "`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openMetadataTestDatabase(t, "missing-auth-index.db")
			var refresh atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/v8/management/credentials":
					fmt.Fprint(w, `{"files":[]}`)
				case "/v8/management/config/api-keys/codex", "/v8/management/config/api-keys/openai-compatibility":
					source := "codex"
					if r.URL.Path == "/v8/management/config/api-keys/openai-compatibility" {
						source = "openai"
					}
					if !refresh.Load() {
						fmt.Fprintf(w, `[{"name":"original","keys":[{"api-key":"old-key","auth_index":%q},{"api-key":"old-key","auth_index":%q}]}]`, source+"-kept", source+"-omitted")
					} else {
						fmt.Fprintf(w, `[{"name":"changed","keys":[{"api-key":"changed-key","auth_index":%q},{"api-key":"do-not-expose-api-key"%s}]}]`, source+"-kept", tc.authField)
					}
				case "/v8/management/config/api-keys/xai":
					key := "old-key"
					if refresh.Load() {
						key = "updated-xai-key"
					}
					fmt.Fprintf(w, `[{"keys":[{"api-key":%q,"auth_index":"xai-auth"}]}]`, key)
				case "/v8/management/config/api-keys/claude":
					if !refresh.Load() {
						fmt.Fprint(w, `[{"keys":[{"api-key":"old-key","auth_index":"claude-auth"}]}]`)
					} else {
						fmt.Fprint(w, `[]`)
					}
				case "/v8/management/config/access/api-keys", "/v8/management/config/api-keys/gemini", "/v8/management/config/api-keys/interactions", "/v8/management/config/api-keys/vertex", "/v8/management/config/api-keys/meta":
					fmt.Fprint(w, `[]`)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			// 真实响应先建立旧身份；混合合法与缺标识条目后，失败来源不得更新或删除任何旧行。
			syncer := service.NewSyncServiceWithClient(db, server.URL, cpa.NewClient(server.URL, "management-secret", time.Second, false))
			if err := syncer.SyncMetadata(context.Background()); err != nil {
				t.Fatal(err)
			}
			before := loadMetadataIdentityMap(t, db)
			refresh.Store(true)
			if err := syncer.SyncMetadata(context.Background()); err == nil || !strings.Contains(err.Error(), "auth_index") || strings.Contains(err.Error(), "do-not-expose-api-key") {
				t.Errorf("refresh warning = %v", err)
			}
			after := loadMetadataIdentityMap(t, db)
			for _, authIndex := range []string{"codex-kept", "codex-omitted", "openai-kept", "openai-omitted"} {
				key := metadataIdentityKey(entities.UsageIdentityAuthTypeAIProvider, authIndex)
				row, ok := after[key]
				if !ok || !reflect.DeepEqual(before[key], row) {
					t.Errorf("%s row changed: deleted = %v, lookup unchanged = %v", authIndex, row.IsDeleted, before[key].LookupKey == row.LookupKey)
				}
			}
			xai := after[metadataIdentityKey(entities.UsageIdentityAuthTypeAIProvider, "xai-auth")]
			claude := after[metadataIdentityKey(entities.UsageIdentityAuthTypeAIProvider, "claude-auth")]
			if xai.IsDeleted || xai.LookupKey != "updated-xai-key" || !claude.IsDeleted {
				t.Error("successful sources did not update or delete their own identities")
			}
		})
	}
}

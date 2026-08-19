package semanticrouter

import "testing"

func TestRealSchedulerDryRunCapabilityPools(t *testing.T) {
	scheduler := NewDefaultRealSchedulerDryRun()
	preRouter := NewPreRouter(NewMultiLayerSemanticRouter(), NewRuleBasedTierRouter(), nil)

	tests := []struct {
		name         string
		model        string
		req          *RouteRequest
		wantPool     PreferredPool
		wantPoolName string
		assertCap    func(*testing.T, *RealSchedulerAccount)
	}{
		{
			name:  "code_pool only selects code-capable account",
			model: "gpt-4",
			req: &RouteRequest{
				Model:  "gpt-4",
				Prompt: "帮我写一个排序算法",
			},
			wantPool:     PoolCode,
			wantPoolName: "code_pool",
			assertCap: func(t *testing.T, account *RealSchedulerAccount) {
				t.Helper()
				if !account.CodeCapable {
					t.Fatalf("selected account %d is not code-capable", account.ID)
				}
			},
		},
		{
			name:  "data_pool only selects data-capable account",
			model: "gpt-4",
			req: &RouteRequest{
				Model:  "gpt-4",
				Prompt: "分析销售数据并做可视化",
			},
			wantPool:     PoolData,
			wantPoolName: "data_pool",
			assertCap: func(t *testing.T, account *RealSchedulerAccount) {
				t.Helper()
				if !account.DataCapable {
					t.Fatalf("selected account %d is not data-capable", account.ID)
				}
			},
		},
		{
			name:  "vision_pool only selects vision-capable account",
			model: "gpt-4o",
			req: &RouteRequest{
				Model:    "gpt-4o",
				Prompt:   "请描述这张图片",
				HasImage: true,
			},
			wantPool:     PoolVision,
			wantPoolName: "vision_pool",
			assertCap: func(t *testing.T, account *RealSchedulerAccount) {
				t.Helper()
				if !account.VisionCapable {
					t.Fatalf("selected account %d is not vision-capable", account.ID)
				}
			},
		},
		{
			name:  "document_pool only selects document-capable account",
			model: "gpt-4",
			req: &RouteRequest{
				Model:        "gpt-4",
				Prompt:       "请总结这个文档",
				HasDocument:  true,
				DocumentType: "docx",
			},
			wantPool:     PoolDocument,
			wantPoolName: "document_pool",
			assertCap: func(t *testing.T, account *RealSchedulerAccount) {
				t.Helper()
				if !account.DocumentCapable {
					t.Fatalf("selected account %d is not document-capable", account.ID)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			preRoute, err := preRouter.Route(nil, tt.model, "", "", tt.req)
			if err != nil {
				t.Fatalf("PreRoute error = %v", err)
			}
			if preRoute.Decision.FinalPool != tt.wantPool {
				t.Fatalf("FinalPool = %v, want %v", preRoute.Decision.FinalPool, tt.wantPool)
			}

			result := scheduler.Select(&SchedulerSelectRequest{
				Model:                tt.model,
				PreferredPool:        preRoute.Decision.FinalPool,
				PreferredTier:        preRoute.Decision.Tier.PreferredTier,
				TaskType:             preRoute.Decision.Semantic.TaskType,
				RequiredCapabilities: preRoute.Decision.Semantic.RequiredCapabilities,
			})
			if result.Error != nil {
				t.Fatalf("dry-run select error = %v", result.Error)
			}
			assertDryRunResult(t, result, tt.wantPoolName)

			account, ok := scheduler.GetAccountByID(result.SelectedAccountID)
			if !ok {
				t.Fatalf("selected account %d not found", result.SelectedAccountID)
			}
			tt.assertCap(t, account)
		})
	}
}

func TestRealSchedulerDryRunSkipsDisabledAndAccountZero(t *testing.T) {
	scheduler := NewRealSchedulerDryRun([]*RealSchedulerAccount{
		{
			ID:               0,
			Name:             "invalid-zero",
			Status:           "active",
			Schedulable:      true,
			Pool:             "code_pool",
			Tier:             TierMedium,
			Model:            "bad-zero",
			CodeCapable:      true,
			ConcurrencyLimit: 1,
		},
		{
			ID:               201,
			Name:             "disabled-code",
			Status:           "disabled",
			Schedulable:      true,
			Pool:             "code_pool",
			Tier:             TierMedium,
			Model:            "disabled-model",
			CodeCapable:      true,
			ConcurrencyLimit: 1,
		},
		{
			ID:               202,
			Name:             "active-code",
			Status:           "active",
			Schedulable:      true,
			Pool:             "code_pool",
			Tier:             TierMedium,
			Model:            "gpt-4.1-mini",
			CodeCapable:      true,
			ConcurrencyLimit: 1,
		},
	})

	result := scheduler.Select(&SchedulerSelectRequest{
		Model:         "gpt-4",
		PreferredPool: PoolCode,
		PreferredTier: TierMedium,
		TaskType:      TaskTypeCode,
	})
	if result.Error != nil {
		t.Fatalf("dry-run select error = %v", result.Error)
	}
	assertDryRunResult(t, result, "code_pool")
	if result.SelectedAccountID != 202 {
		t.Fatalf("SelectedAccountID = %d, want active account 202", result.SelectedAccountID)
	}
}

func assertDryRunResult(t *testing.T, result *SchedulerSelectResult, wantPool string) {
	t.Helper()
	if result.SelectedAccountID == 0 {
		t.Fatalf("selected_account_id must not be 0")
	}
	if result.SelectedModel == "" {
		t.Fatalf("selected_model is empty")
	}
	if result.PoolUsed != wantPool {
		t.Fatalf("selected_pool = %s, want %s", result.PoolUsed, wantPool)
	}
	if result.Layer != "dry_run_load_balance" {
		t.Fatalf("scheduler_layer = %s, want dry_run_load_balance", result.Layer)
	}
}

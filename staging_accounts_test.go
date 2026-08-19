package semanticrouter

import (
	"context"
	"testing"
)

func TestStagingRepositoryPoolCandidates(t *testing.T) {
	scheduler := NewStagingRealScheduler(1001)
	cases := []struct {
		name       string
		pool       PreferredPool
		tier       PreferredTier
		task       TaskType
		capability RequiredCapabilities
		wantPool   string
	}{
		{"code", PoolCode, TierStrong, TaskTypeCode, RequiredCapabilities{}, "code_pool"},
		{"data", PoolData, TierStrong, TaskTypeText, RequiredCapabilities{}, "data_pool"},
		{"vision", PoolVision, TierStrong, TaskTypeVision, RequiredCapabilities{VisionCapable: true}, "vision_pool"},
		{"document", PoolDocument, TierStrong, TaskTypeDocument, RequiredCapabilities{DocumentCapable: true}, "document_pool"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := scheduler.Select(&SchedulerSelectRequest{PreferredPool: tc.pool, PreferredTier: tc.tier, TaskType: tc.task, RequiredCapabilities: tc.capability})
			if result.Error != nil {
				t.Fatal(result.Error)
			}
			if result.SelectedAccountID == 0 || result.PoolUsed != tc.wantPool {
				t.Fatalf("unexpected staging selection: %+v", result)
			}
			if result.SelectedAccountID == 1701 || result.SelectedAccountID == 1702 {
				t.Fatalf("disabled/unschedulable account selected: %+v", result)
			}
		})
	}
	if accounts, err := NewStagingAccountRepository().ListRoutingAccounts(context.Background(), ptrInt64(1001)); err != nil || len(accounts) != 17 {
		t.Fatalf("staging repository accounts = %d, err=%v", len(accounts), err)
	}
}

func ptrInt64(value int64) *int64 { return &value }

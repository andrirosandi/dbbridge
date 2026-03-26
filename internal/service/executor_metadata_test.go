package service

import (
	"testing"
)

func TestProcessSelectBlock_Basic(t *testing.T) {
	executor := &QueryExecutor{}

	tests := []struct {
		name           string
		sql            string
		expectHasBlock bool
		expectCountSQL string
		expectContent  string
	}{
		{
			name:           "no select block",
			sql:            "SELECT * FROM orders",
			expectHasBlock: false,
			expectCountSQL: "",
		},
		{
			name:           "with select block",
			sql:            "SELECT {select}id, name{endselect} FROM users",
			expectHasBlock: true,
			expectCountSQL: "SELECT COUNT(*) FROM users",
			expectContent:  "id, name",
		},
		{
			name:           "with select block and complex query",
			sql:            "begin select {select}id, trdate, amount{endselect} from #trx end",
			expectHasBlock: true,
			expectCountSQL: "begin select COUNT(*) from #trx end",
			expectContent:  "id, trdate, amount",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := executor.processSelectBlock(tt.sql)
			if result.HasBlock != tt.expectHasBlock {
				t.Errorf("HasBlock = %v, want %v", result.HasBlock, tt.expectHasBlock)
			}
			if result.CountSQL != tt.expectCountSQL {
				t.Errorf("CountSQL = %q, want %q", result.CountSQL, tt.expectCountSQL)
			}
			if tt.expectHasBlock && result.SelectContent != tt.expectContent {
				t.Errorf("SelectContent = %q, want %q", result.SelectContent, tt.expectContent)
			}
		})
	}
}

func TestBuildMetadata(t *testing.T) {
	executor := &QueryExecutor{}

	tests := []struct {
		name             string
		page             int
		limit            int
		total            int64
		expectTotalPages int
		expectHasNext    bool
		expectHasPrev    bool
	}{
		{
			name:             "page 1 of 8",
			page:             1,
			limit:            20,
			total:            150,
			expectTotalPages: 8,
			expectHasNext:    true,
			expectHasPrev:    false,
		},
		{
			name:             "page 5 of 8",
			page:             5,
			limit:            20,
			total:            150,
			expectTotalPages: 8,
			expectHasNext:    true,
			expectHasPrev:    true,
		},
		{
			name:             "last page",
			page:             8,
			limit:            20,
			total:            150,
			expectTotalPages: 8,
			expectHasNext:    false,
			expectHasPrev:    true,
		},
		{
			name:             "empty result",
			page:             1,
			limit:            20,
			total:            0,
			expectTotalPages: 0,
			expectHasNext:    false,
			expectHasPrev:    false,
		},
		{
			name:             "single page",
			page:             1,
			limit:            20,
			total:            15,
			expectTotalPages: 1,
			expectHasNext:    false,
			expectHasPrev:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta := executor.buildMetadata(tt.page, tt.limit, tt.total)

			if *meta.TotalPages != tt.expectTotalPages {
				t.Errorf("TotalPages = %d, want %d", *meta.TotalPages, tt.expectTotalPages)
			}
			if *meta.HasNext != tt.expectHasNext {
				t.Errorf("HasNext = %v, want %v", *meta.HasNext, tt.expectHasNext)
			}
			if *meta.HasPrev != tt.expectHasPrev {
				t.Errorf("HasPrev = %v, want %v", *meta.HasPrev, tt.expectHasPrev)
			}
			if *meta.Page != tt.page {
				t.Errorf("Page = %d, want %d", *meta.Page, tt.page)
			}
			if *meta.Limit != tt.limit {
				t.Errorf("Limit = %d, want %d", *meta.Limit, tt.limit)
			}
			if *meta.Total != tt.total {
				t.Errorf("Total = %d, want %d", *meta.Total, tt.total)
			}
		})
	}
}

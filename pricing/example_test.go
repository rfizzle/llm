package pricing_test

import (
	"fmt"

	"github.com/rfizzle/llm/pricing"
)

// ExampleTable_Price prices one request from the counts a provider reports.
// The prompt count includes the cached tokens, as llm.Usage's does.
func ExampleTable_Price() {
	table := pricing.NewTable(map[string]pricing.ModelPricing{
		"demo-model": {InputCostPerToken: 2e-6, OutputCostPerToken: 8e-6, CacheReadCostPerToken: 2e-7},
	})
	cost, found := table.Price("demo-model", 10_000, 500, 6_000, 0)
	fmt.Printf("$%.4f %v\n", cost, found)
	_, found = table.Price("unknown-model", 10_000, 500, 0, 0)
	fmt.Println(found)
	// Output:
	// $0.0132 true
	// false
}

// ExampleSnapshotJSON seeds a cache directory with the table built into the
// library, the floor a download is laid over.
func ExampleSnapshotJSON() {
	fmt.Println(len(pricing.SnapshotJSON()) > 0, pricing.Snapshot().Len() > 0)
	// Output: true true
}

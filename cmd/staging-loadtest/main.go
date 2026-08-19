package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	semanticrouter "semantic-router"
	"time"
)

func main() {
	address := flag.String("address", "127.0.0.1:9101", "staging shadow TCP address")
	requests := flag.Int("requests", 1000, "number of dry-run requests")
	concurrency := flag.Int("concurrency", 16, "parallel workers")
	groupID := flag.Int64("group-id", 1001, "staging account group")
	flag.Parse()
	config := semanticrouter.DefaultIntegrationConfig()
	config.ServiceAddress = *address
	config.ConnectTimeout = 2 * time.Second
	config.RequestTimeout = 2 * time.Second
	result, err := semanticrouter.RunStagingLoadTest(context.Background(), semanticrouter.NewModelSelectorTCPClient(config), semanticrouter.StagingLoadTestConfig{Requests: *requests, Concurrency: *concurrency, GroupID: *groupID})
	if err != nil {
		panic(err)
	}
	data, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(data))
}

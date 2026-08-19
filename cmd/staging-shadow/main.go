package main

import (
	"log"
	"os"
	"os/signal"
	semanticrouter "semantic-router"
	"strconv"
	"syscall"
)

func main() {
	config := semanticrouter.DefaultIntegrationConfig()
	if value, ok := os.LookupEnv("SEMANTIC_ROUTER_TCP_LISTEN_ADDRESS"); ok {
		config.ListenAddress = value
		config.ServiceAddress = value
	}
	groupID := int64(1001)
	if value, ok := os.LookupEnv("SEMANTIC_ROUTER_STAGING_GROUP_ID"); ok {
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil && parsed != 0 {
			groupID = parsed
		}
	}
	service, err := semanticrouter.NewModelSelectionService(semanticrouter.NewStagingRealScheduler(groupID), config)
	if err != nil {
		log.Fatal(err)
	}
	server, err := semanticrouter.NewModelSelectorTCPServer(service, config)
	if err != nil {
		log.Fatal(err)
	}
	if err := server.Start(); err != nil {
		log.Fatal(err)
	}
	defer server.Close()
	log.Printf("staging shadow selector listening on %s", server.Addr())
	log.Printf("group_id=%d shadow_only=%t dry_run=%t takeover=%t upstream_called=false", groupID, config.ShadowEnabled, config.DryRunEnabled, config.TakeoverEnabled)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	<-signals
}

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
	flag.Parse()
	config := semanticrouter.DefaultIntegrationConfig()
	config.ServiceAddress = *address
	config.ConnectTimeout = 2 * time.Second
	config.RequestTimeout = 2 * time.Second
	client := semanticrouter.NewModelSelectorTCPClient(config)
	cases := []semanticrouter.ModelSelectionRequest{
		{ProtocolVersion: semanticrouter.ModelSelectorProtocolVersion, RequestID: "sim-code", GroupID: 1001, Prompt: "implement a Python login API"},
		{ProtocolVersion: semanticrouter.ModelSelectorProtocolVersion, RequestID: "sim-data", GroupID: 1001, Prompt: "analyze this CSV and generate a trend chart"},
		{ProtocolVersion: semanticrouter.ModelSelectorProtocolVersion, RequestID: "sim-vision", GroupID: 1001, Prompt: "describe the objects in this image", HasImage: true},
		{ProtocolVersion: semanticrouter.ModelSelectorProtocolVersion, RequestID: "sim-document", GroupID: 1001, Prompt: "summarize this PDF document", HasDocument: true},
		{ProtocolVersion: semanticrouter.ModelSelectorProtocolVersion, RequestID: "sim-chat", GroupID: 1001, Prompt: "你好，请介绍一下自己"},
		{ProtocolVersion: semanticrouter.ModelSelectorProtocolVersion, RequestID: "sim-image", GroupID: 1001, Prompt: "generate a futuristic city image"},
	}
	results := make([]interface{}, 0, len(cases))
	for _, request := range cases {
		response, err := client.Select(context.Background(), &request)
		results = append(results, map[string]interface{}{"request_id": request.RequestID, "response": response, "error": errorString(err)})
	}
	data, _ := json.MarshalIndent(results, "", "  ")
	fmt.Println(string(data))
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

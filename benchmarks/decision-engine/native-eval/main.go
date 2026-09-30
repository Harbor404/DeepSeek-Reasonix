package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"reasonix/internal/tools/decision"
)

func main() {
	checking := flag.Bool("check", false, "Validate a candidate answer and return canonical structured delivery")
	runtime := flag.Bool("runtime", false, "Read NDJSON compare/check/update/deliver commands")
	store := flag.String("store", "", "Host-selected SQLite path for persistent runtime snapshots")
	flag.Parse()
	if *runtime {
		runRuntime(*store)
		return
	}
	raw, err := io.ReadAll(io.LimitReader(os.Stdin, 256*1024+1))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	executor := decision.New()
	if *checking {
		executor = decision.NewCheck()
	}
	out, err := executor.Execute(context.Background(), raw)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(out)
}

func runRuntime(path string) {
	tools := decision.NewRuntime()
	if path != "" {
		tools = decision.NewPersistentRuntime(path)
	}
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4096), 512*1024)
	for scanner.Scan() {
		var command struct {
			Action    string          `json:"action"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &command); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		index := 0
		switch command.Action {
		case "compare":
		case "check":
			index = 1
		case "update":
			index = 2
		case "deliver":
			index = 3
		default:
			fmt.Fprintln(os.Stderr, "runner.action_invalid")
			os.Exit(1)
		}
		result, err := tools[index].Execute(context.Background(), command.Arguments)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(result)
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

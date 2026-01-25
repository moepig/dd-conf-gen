package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/moepig/dd-conf-gen/checks/redisdb"
)

func main() {
	// コマンドライン引数の定義
	configPath := flag.String("config", "", "Path to configuration file")
	checkType := flag.String("check", "", "Check type")
	flag.Parse()

	// check オプションの検証
	if *checkType == "" {
		fmt.Fprintln(os.Stderr, "Error: -check option is required")
		flag.Usage()
		os.Exit(1)
	}

	ctx := context.Background()

	// チェックタイプに応じた処理
	switch *checkType {
	case "redisdb":
		// Redis チェック設定の生成
		yamlData, err := redisdb.Run(ctx, *configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Print(string(yamlData))
	default:
		fmt.Fprintf(os.Stderr, "Error: unsupported check type: %s \n", *checkType)
		os.Exit(1)
	}
}

package main

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
)

type config struct {
	addr      string
	database  string
	selfcheck bool
}

func parseConfig(args []string) (config, error) {
	set := flag.NewFlagSet("cleanroom-monitor-release", flag.ContinueOnError)
	var cfg config
	set.StringVar(&cfg.addr, "addr", "", "HTTP 监听地址")
	set.StringVar(&cfg.database, "db", "cleanroom-monitor.db", "SQLite 数据库路径或 DSN")
	set.BoolVar(&cfg.selfcheck, "selfcheck", false, "运行有界端到端自检")
	if err := set.Parse(args); err != nil {
		return cfg, err
	}
	if set.NArg() != 0 {
		return cfg, errors.New("不接受位置参数")
	}
	if cfg.addr == "" {
		cfg.addr = "127.0.0.1:19091"
		if value := os.Getenv("PORT"); value != "" {
			port, err := strconv.Atoi(value)
			if err != nil || port < 1 || port > 65535 {
				return cfg, fmt.Errorf("PORT 必须是 1 到 65535 的端口号")
			}
			cfg.addr = net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
		}
	}
	host, port, err := net.SplitHostPort(cfg.addr)
	if err != nil || host == "" || port == "" {
		return cfg, fmt.Errorf("-addr 必须是 host:port: %w", err)
	}
	return cfg, nil
}

package config

import "os"

type Config struct {
	Port                string
	MySQLDSN            string
	CORSOrigin          string
	CollectorUserAgent  string
	EnableHLTVCollector bool
	TranslationURL      string
}

func Load() Config {
	return Config{
		Port:                env("APP_PORT", "8080"),
		MySQLDSN:            env("MYSQL_DSN", "cs_pulse:cs_pulse_dev@tcp(127.0.0.1:3306)/cs_pulse?charset=utf8mb4&parseTime=True&loc=Local"),
		CORSOrigin:          env("CORS_ORIGIN", "http://localhost:5173"),
		CollectorUserAgent:  env("COLLECTOR_USER_AGENT", "CSPulseLearning/0.1 (local learning project)"),
		EnableHLTVCollector: env("ENABLE_HLTV_COLLECTOR", "true") == "true",
		TranslationURL:      env("TRANSLATION_URL", ""),
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

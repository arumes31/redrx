package config

import (
	"log"

	"github.com/spf13/viper"
)

type Config struct {
	AppEnv            string `mapstructure:"APP_ENV"`
	Port              string `mapstructure:"PORT"`
	DatabaseURL       string `mapstructure:"DATABASE_URL"`
	RedisURL          string `mapstructure:"REDIS_URL"`
	MaxMindAccountID  string `mapstructure:"MAXMIND_ACCOUNT_ID"`
	MaxMindLicenseKey string `mapstructure:"MAXMIND_LICENSE_KEY"`
	MaxMindEditionIDs string `mapstructure:"MAXMIND_EDITION_IDS"`
	MaxMindDBPath     string `mapstructure:"GEOIP_DB_PATH"`
}

func LoadConfig() (config Config, err error) {
	viper.SetDefault("APP_ENV", "local")
	viper.SetDefault("PORT", "8080")
	viper.SetDefault("DATABASE_URL", "postgresql://redrx:securepassword@localhost:5432/redrx_db?sslmode=disable")
	viper.SetDefault("REDIS_URL", "redis://localhost:6379")
	viper.SetDefault("GEOIP_DB_PATH", "./geoip/GeoLite2-Country.mmdb")
	viper.SetDefault("MAXMIND_EDITION_IDS", "GeoLite2-Country")

	viper.AutomaticEnv()

	err = viper.Unmarshal(&config)
	if err != nil {
		log.Printf("unable to decode into struct, %v", err)
		return
	}

	return
}

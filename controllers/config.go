package controllers

import (
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type config struct {
}

func getConf() *config {
	viper.SetConfigFile("config.yaml")
	err := viper.ReadInConfig()

	if err != nil {
		log.Error(err)
	}

	conf := &config{}
	err = viper.Unmarshal(conf)
	if err != nil {
		log.Error(err, "unable to decode into config struct")
	}

	return conf
}

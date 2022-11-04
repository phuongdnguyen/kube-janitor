package main

import (
	"context"
	"flag"
	"path/filepath"

	"github.com/phuongnd96/kube-janitor/api"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// Create a new config instance.
var (
	conf *api.Config
)

// Read the config file from the current directory and marshal
// into the conf config struct.
func getConf() *api.Config {
	viper.SetConfigFile("config/config.yaml")
	err := viper.ReadInConfig()

	if err != nil {
		log.Error(err)
	}

	conf := &api.Config{}
	err = viper.Unmarshal(conf)
	if err != nil {
		log.Error(err, "unable to decode into config struct")
	}

	return conf
}

// Initialization routine.
func init() {
	// Retrieve config options.
	conf = getConf()
}

// Main program.
func main() {
	ctx := context.Background()
	var kubeconfig *string

	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		log.Printf("Building config from flags failed, %s, trying to build inclusterconfig", err.Error())
		config, err = rest.InClusterConfig()
		if err != nil {
			log.Printf("error %s building inclusterconfig", err.Error())
		}
	}
	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Error(err)
	}

}

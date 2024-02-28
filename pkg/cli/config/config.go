package config

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	logger "github.com/sirupsen/logrus"
	"github.com/spf13/viper"

	"github.com/openshift/backplane-cli/pkg/info"
	"github.com/openshift/backplane-cli/pkg/ocm"
)

type BackplaneConfiguration struct {
	URL              string
	ProxyURL         *string // Optional
	SessionDirectory string
	AssumeInitialArn string
}

// GetConfigFilePath returns the Backplane CLI configuration filepath
func GetConfigFilePath() (string, error) {
	// Check if user has explicitly defined backplane config path
	path, found := os.LookupEnv(info.BackplaneConfigPathEnvName)
	if found {
		return path, nil
	}

	UserHomeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	configFilePath := filepath.Join(UserHomeDir, info.BackplaneConfigDefaultFilePath, info.BackplaneConfigDefaultFileName)

	return configFilePath, nil
}

// GetBackplaneConfiguration parses and returns the given backplane configuration
func GetBackplaneConfiguration() (bpConfig BackplaneConfiguration, err error) {
	filePath, err := GetConfigFilePath()
	if err != nil {
		return bpConfig, err
	}

	viper.AutomaticEnv()

	// Check if the config file exists
	if _, err = os.Stat(filePath); err == nil {
		// Load config file
		viper.SetConfigFile(filePath)
		viper.SetConfigType("json")

		if err := viper.ReadInConfig(); err != nil {
			return bpConfig, err
		}
	}

	// Check if user has explicitly defined proxy; it has higher precedence over the config file
	err = viper.BindEnv("proxy-url", info.BackplaneProxyEnvName)
	if err != nil {
		return bpConfig, err
	}

	// Warn user if url defined in the config file
	if viper.GetString("url") != "" {
		logger.Warn("Manual URL configuration is deprecated, please remove URL key from Backplane configuration")
	}

	// Check if user has explicitly defined backplane URL via env; it has higher precedence over the ocm env URL
	url, ok := getBackplaneEnv(info.BackplaneURLEnvName)
	if ok {
		bpConfig.URL = url
	} else {
		// Fetch backplane URL from ocm env
		if bpConfig.URL, err = bpConfig.GetBackplaneURL(); err != nil {
			return bpConfig, err
		}
	}

	bpConfig.SessionDirectory = viper.GetString("session-dir")
	bpConfig.AssumeInitialArn = viper.GetString("assume-initial-arn")

	// proxyURL is optional
	proxyInConfigFile := viper.GetStringSlice("proxy-url")
	proxyURL := bpConfig.getFirstWorkingProxyURL(proxyInConfigFile)
	if proxyURL != "" {
		bpConfig.ProxyURL = &proxyURL
	} else {
		logger.Warn("No proxy configuration available. This may result in failing commands as backplane-api is only available from select networks.")
	}
	return bpConfig, nil
}

// verifyBackplaneConfiguration checks the backplane configuration datastructure for missing fields
// doesn't return anything other than some warning messages in the logger to help with diagnostics
func VerifyBackplaneConfiguration(bpConfig BackplaneConfiguration) bool {
	urlString := bpConfig.URL
	sessionDirectoryString := bpConfig.SessionDirectory
	AssumeInitialArnString := bpConfig.AssumeInitialArn

	urlStringLen := len(urlString)
	sessionDirectoryLen := len(sessionDirectoryString)
	AssumeInitialArnStringLen := len(AssumeInitialArnString)

	missing := ""

	logger.Info("Validating backplane config fields...\n")

	if urlStringLen == 0 {
		missing = missing + " url "
		logger.Warn("url in backplane configurations is either empty or undefined, please define the field url or use the environment varible BACKPLANE_URL")
	}
	// This is usually not a problem
	if sessionDirectoryLen == 0 {
		logger.Info("session-dir in backplane config is either empty or undefined")
	}
	if AssumeInitialArnStringLen == 0 {
		missing = missing + " assume-initial-arn "
		logger.Warn("assume-initial-arn in backplane configurations is either empty or undefined, please define the field assume-initial-arn in config")
	}

	// For mandatory fields only
	// e.g url and arn
	if urlStringLen == 0 || AssumeInitialArnStringLen == 0 {
		fmt.Printf("Attention The backplane configuration file or environment varibles are missing some important fields:%v", missing)
		fmt.Println("Your backplane CLI Config as a best practice should contain")
		fmt.Println("{")
		fmt.Println("proxy-url: <proxy url in quotes>")
		fmt.Println("assume-initial-arn: <arn in quotes>")
		fmt.Println("}")
		fmt.Println("NOTE: backplane url, arn is usually extracted via ocm")
		fmt.Println("If you need to manually define a backplane url, set an environment varible called BACKPLANE_URL")
		return false
	} else {
		logger.Info("Config Fields Checks Complete")
	}
	return true
}

var clientDo = func(client *http.Client, req *http.Request) (*http.Response, error) {
	return client.Do(req)
}

func (config *BackplaneConfiguration) getFirstWorkingProxyURL(s []string) string {
	bpURL := config.URL + "/healthz"

	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	for _, p := range s {
		proxyURL, err := url.ParseRequestURI(p)
		if err != nil {
			logger.Debugf("proxy-url: '%v' could not be parsed.", p)
			continue
		}

		client.Transport = &http.Transport{Proxy: http.ProxyURL(proxyURL)}
		req, _ := http.NewRequest("GET", bpURL, nil)
		resp, err := clientDo(client, req)

		if err != nil {
			logger.Infof("Proxy: %s returned an error: %s", proxyURL, err)
			continue
		}
		if resp.StatusCode == http.StatusOK {
			return p
		}
	}
	return ""
}

func GetConfigDirctory() (string, error) {
	bpConfigFilePath, err := GetConfigFilePath()
	if err != nil {
		return "", err
	}
	configDirectory := filepath.Dir(bpConfigFilePath)

	return configDirectory, nil
}

// GetBackplaneURL returns API URL
func (config *BackplaneConfiguration) GetBackplaneURL() (string, error) {

	ocmEnv, err := ocm.DefaultOCMInterface.GetOCMEnvironment()
	if err != nil {
		return "", err
	}
	url, ok := ocmEnv.GetBackplaneURL()
	if !ok {
		return "", fmt.Errorf("the requested API endpoint is not available for the OCM environment: %v", ocmEnv.Name())
	}
	logger.Infof("Backplane URL retrieved via OCM environment: %s", url)
	return url, nil
}

// getBackplaneEnv retrieves the value of the environment variable named by the key
func getBackplaneEnv(key string) (string, bool) {
	val, ok := os.LookupEnv(key)
	if ok {
		logger.Infof("Backplane key %s set via env vars: %s", key, val)
		return val, ok
	}
	return "", false
}

// CheckAPIConnection validate API connection via configured proxy and VPN
func (config BackplaneConfiguration) CheckAPIConnection() error {

	// make test api connection
	connectionOk, err := config.testHTTPRequestToBackplaneAPI()

	if !connectionOk {
		return err
	}

	return nil
}

// testHTTPRequestToBackplaneAPI returns status of the API connection
func (config BackplaneConfiguration) testHTTPRequestToBackplaneAPI() (bool, error) {
	client := http.Client{
		Timeout: 5 * time.Second,
	}

	if config.ProxyURL != nil {
		proxyURL, err := url.Parse(*config.ProxyURL)
		if err != nil {
			return false, err
		}
		http.DefaultTransport = &http.Transport{Proxy: http.ProxyURL(proxyURL)}
	}

	req, err := http.NewRequest("HEAD", config.URL, nil)
	if err != nil {
		return false, err
	}
	_, err = client.Do(req)
	if err != nil {
		return false, err
	}

	return true, nil
}

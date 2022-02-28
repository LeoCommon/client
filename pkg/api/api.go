package api

import (
	"crypto/tls"
	"fmt"
	"time"

	"github.com/go-resty/resty/v2"
)

type ProvisioningAPI struct {
	client *resty.Client
}

func SetupAPI(baseURL string, rootCert string) (api *ProvisioningAPI) {
	client := resty.New()

	client.
		// Set up the api base-url
		SetBaseURL(baseURL).
		// Set up the certificate authentification
		SetRootCertificate(rootCert).
		SetRetryCount(3).
		SetRetryMaxWaitTime(10 * time.Second)

	return &ProvisioningAPI{client}
}

func (api *ProvisioningAPI) LoadClientCertificates(certFile string, keyFile string) error {
	// Load our client certificate
	clientCert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return err
	}

	api.LoadClientCertificate(clientCert)
	return nil
}

func (api *ProvisioningAPI) LoadClientCertificate(clientCert tls.Certificate) {
	api.client.SetCertificates(clientCert)
}

func (api *ProvisioningAPI) IsAdopted() {
	resp, err := api.client.R().Get("get")

	fmt.Printf("resp %v, error %v", resp, err)
}

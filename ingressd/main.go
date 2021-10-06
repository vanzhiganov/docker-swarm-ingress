package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/docker/cli/cli/connhelper"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
)

type ServiceEntry struct {
	ServiceName        string
	ServiceDomain      string
	ServicePath        string
	ServicePort        string
	ServiceSSL         bool
	ServiceRedirectSSL bool
}

type IngressConfig struct {
	UpdateInterval int
	OutputFile     string
	templateFile   string
}

type Ingress struct {
	DockerClient    *client.Client
	OutputFile      string
	ServiceEntries  []ServiceEntry
	ServiceTemplate *template.Template
}

func GetEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if len(value) == 0 {
		return defaultValue
	}
	return value
}

func NewIngress(outputFile string, templateFile string) *Ingress {
	var clientOpts []client.Opt

	helper, err := connhelper.GetConnectionHelper(os.Getenv("DOCKER_HOST"))

	if err != nil {
		panic(err)
	}

	if helper != nil {
		httpClient := &http.Client{
			Transport: &http.Transport{
				DialContext: helper.Dialer,
			},
		}

		clientOpts = append(clientOpts,
			client.WithHTTPClient(httpClient),
			client.WithHost(helper.Host),
			client.WithDialContext(helper.Dialer),
		)
	}

	client, err := client.NewClientWithOpts(clientOpts...)
	if err != nil {
		panic(err)
	}

	return &Ingress{
		DockerClient:    client,
		OutputFile:      outputFile,
		ServiceTemplate: template.Must(template.ParseFiles(templateFile)),
	}
}

func (s *Ingress) StartProxyServer() {
	cmd := exec.Command("/usr/local/openresty/bin/openresty")
	err := cmd.Start()
	if err != nil {
		panic(err)
	}

	// use goroutine waiting, manage process
	// this is important, otherwise the process becomes in S mode
	go func() {
		err = cmd.Wait()
		fmt.Printf("Command finished with error: %v", err)
	}()
}

func (s *Ingress) ReloadProxyServer() {
	outputCmd, err := exec.Command("/usr/local/openresty/bin/openresty", "-s", "reload").Output()
	if err != nil {
		fmt.Printf("Failed to execute command: %s", outputCmd)
		fmt.Println(err)
	}
}

func (s *Ingress) GenerateTemplate() {
	if _, err := os.Stat(string(s.OutputFile)); err == nil {
		err = os.Remove(string(s.OutputFile))
		if err != nil {
			fmt.Println(err)
			return
		}
	}

	f, err := os.Create(string(s.OutputFile))
	if err != nil {
		fmt.Println("Create file Error: ", err)
		return
	}

	if err := s.ServiceTemplate.Execute(f, s.ServiceEntries); err != nil {
		fmt.Println(err)
	}
}

func (s *Ingress) GetServices() {
	services, err := s.DockerClient.ServiceList(context.Background(), types.ServiceListOptions{})
	if err != nil {
		panic(err)
	}

	for _, svc := range services {
		servicePath := "/"
		servicePort := "80"
		serviceSSL := false
		serviceRedirectSSL := false

		if val, ok := svc.Spec.Labels["ingress.path"]; ok {
			servicePath = val
		}

		if val, ok := svc.Spec.Labels["ingress.port"]; ok {
			servicePort = val
		}

		if val, ok := svc.Spec.Labels["ingress.ssl"]; ok {
			if val == "yes" {
				serviceSSL = true
			}
		}

		if val, ok := svc.Spec.Labels["ingress.ssl_redirect"]; ok {
			if val == "yes" {
				serviceRedirectSSL = true
			}
		}

		domainKeys := make([]string, 0, len(svc.Spec.Labels))

		for key := range svc.Spec.Labels {
			if strings.HasPrefix(key, "ingress.host") {
				domainKeys = append(domainKeys, key)
			}
		}

		for _, domainKey := range domainKeys {
			if val, ok := svc.Spec.Labels[domainKey]; ok {
				entry := &ServiceEntry{
					ServiceName:        svc.Spec.Name,
					ServiceDomain:      val,
					ServicePath:        servicePath,
					ServicePort:        servicePort,
					ServiceSSL:         serviceSSL,
					ServiceRedirectSSL: serviceRedirectSSL,
				}

				s.ServiceEntries = append(s.ServiceEntries, *entry)
			}
		}
	}
}

func main() {
	templateFile := GetEnv("TEMPLATE_FILE", "ingress.tpl")
	outputFile := GetEnv("OUTPUT_FILE", "proxy.conf")
	updateInterval, err := strconv.ParseInt(GetEnv("UPDATE_INTERVAL", "1"), 10, 64)
	if err != nil {
		fmt.Println("Wrong UPDATE_INTERVAL value:")
		fmt.Println(err)
	}

	ingress := NewIngress(outputFile, templateFile)

	ingress.StartProxyServer()

	for {
		ingress.GetServices()
		ingress.GenerateTemplate()
		ingress.ReloadProxyServer()

		time.Sleep(time.Duration(updateInterval) * time.Second)
	}
}

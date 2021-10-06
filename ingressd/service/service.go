package service

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/client"
)

// Service struct
type Service struct {
	DockerClient *client.Client
}

// ServiceStatus structure
type ServiceStatus struct {
	ID              string `json:",omitempty"`
	Name            string
	Err             string              `json:",omitempty"`
	TaskStatus      []TaskStatus        `json:",omitempty"`
	Replicas        *uint64             `json:",omitempty"`
	RunningReplicas int                 `json:",omitempty"`
	FailedReplicas  int                 `json:",omitempty"`
	UpdateStatus    *swarm.UpdateStatus `json:",omitempty"`
}

// TaskStatus structure
type TaskStatus struct {
	TaskID       string          `json:",omitempty"`
	Timestamp    time.Time       `json:",omitempty"`
	DesiredState swarm.TaskState `json:",omitempty"`
	State        swarm.TaskState `json:",omitempty"`
	Message      string          `json:",omitempty"`
	Err          string          `json:",omitempty"`
	Image        string          `json:",omitempty"`
}

// ServiceEntry structure
type ServiceEntry struct {
	ServiceName        string
	ServiceDomain      string
	ServicePath        string
	ServicePort        string
	ServiceSSL         bool
	ServiceRedirectSSL bool
}

// Services defines interfaces with the required methods
type Services interface {
	GetService(filter filters.Args) (swarm.Service, error)
	GetTask(filter filters.Args) ([]swarm.Task, error)
	GetDeploymentStatus(serviceName string, image string) (ServiceStatus, error)
	GetServiceStatus(serviceName string) (ServiceStatus, error)
}

// NewService returns a new instance of the Service structure
func NewService(client *client.Client) *Service {
	return &Service{
		client,
	}
}

// GetService returns swarm.Service struct
func (s *Service) GetService(filter filters.Args) (swarm.Service, error) {
	serviceList, err := s.DockerClient.ServiceList(context.Background(), types.ServiceListOptions{Filters: filter})

	swarmService := swarm.Service{}
	if err != nil {
		return swarmService, err
	}

	for _, service := range serviceList {
		swarmService = service
	}

	return swarmService, nil
}

// GetTask returns the tasks related to a specific service id
func (s *Service) GetTask(filter filters.Args) ([]swarm.Task, error) {
	tasks, err := s.DockerClient.TaskList(context.Background(), types.TaskListOptions{Filters: filter})

	if err != nil {
		return []swarm.Task{}, err
	}

	return tasks, nil
}

// GetDeploymentStatus returns the information about a service and it verifies if the tasks are running
// or for some reason it failed
func (s *Service) GetDeploymentStatus(serviceName string, image string) (ServiceStatus, error) {
	filterService := filters.NewArgs()
	filterService.Add("name", serviceName)
	swarmService, err := s.GetService(filterService)

	deploymentStatus := ServiceStatus{}
	if err != nil {
		return deploymentStatus, err
	}

	deploymentStatus.Name = serviceName

	if swarmService.ID == "" {
		deploymentStatus.Err = fmt.Sprintf("The %s service was not found in the cluster.", serviceName)
		return deploymentStatus, nil
	}

	filterTask := filters.NewArgs()
	filterTask.Add("service", swarmService.ID)

	swarmTask, err := s.GetTask(filterTask)
	if err != nil {
		return deploymentStatus, err
	}

	deploymentStatus.ID = swarmService.ID

	if s.isImageDeploy(swarmTask, image) == false {
		deploymentStatus.Err = fmt.Sprintf("The %s image was not deployed or not found in the current tasks running.", image)
		return deploymentStatus, nil
	}

	deploymentStatus.Replicas = swarmService.Spec.Mode.Replicated.Replicas
	deploymentStatus.TaskStatus = s.parseTaskState(swarmTask)
	deploymentStatus.UpdateStatus = swarmService.UpdateStatus

	deploymentStatus.RunningReplicas, deploymentStatus.FailedReplicas = s.taskStateCount(deploymentStatus, image)

	if deploymentStatus.FailedReplicas > deploymentStatus.RunningReplicas && uint64(deploymentStatus.RunningReplicas) < *deploymentStatus.Replicas {
		deploymentStatus.Err = fmt.Sprintf("Looks like something went wrong during the deployment, because the %s service failed %d time(s) since last deployment", serviceName, deploymentStatus.FailedReplicas)
	}

	if deploymentStatus.UpdateStatus != nil && (deploymentStatus.UpdateStatus.State == swarm.UpdateStatePaused || deploymentStatus.UpdateStatus.State == swarm.UpdateStateRollbackCompleted || deploymentStatus.UpdateStatus.State == swarm.UpdateStateRollbackPaused) {
		deploymentStatus.Err = fmt.Sprintf("Something went wrong during the deployment of the %s service. The error message is: %s", serviceName, deploymentStatus.UpdateStatus.Message)
	}

	return deploymentStatus, nil
}

// GetServiceStatus returns the information about a service and it verifies if the tasks are running
// or for some reason it failed
func (s *Service) GetServiceStatus(serviceName string) (ServiceStatus, error) {
	filterService := filters.NewArgs()
	filterService.Add("name", serviceName)
	swarmService, err := s.GetService(filterService)

	serviceStatus := ServiceStatus{}
	if err != nil {
		return serviceStatus, err
	}

	serviceStatus.Name = serviceName

	if swarmService.ID == "" {
		serviceStatus.Err = fmt.Sprintf("The %s service was not found in the cluster.", serviceName)
		return serviceStatus, nil
	}

	filterTask := filters.NewArgs()
	filterTask.Add("service", swarmService.ID)
	filterTask.Add("desired-state", "running")

	swarmTask, err := s.GetTask(filterTask)
	if err != nil {
		return serviceStatus, err
	}

	serviceStatus.ID = swarmService.ID

	serviceStatus.Replicas = swarmService.Spec.Mode.Replicated.Replicas
	serviceStatus.TaskStatus = s.parseTaskState(swarmTask)
	serviceStatus.UpdateStatus = swarmService.UpdateStatus

	serviceStatus.RunningReplicas, serviceStatus.FailedReplicas = s.taskStateCount(serviceStatus, "")

	return serviceStatus, nil
}

func (s *Service) parseTaskState(swarmTask []swarm.Task) []TaskStatus {
	taskStatus := []TaskStatus{}
	for _, task := range swarmTask {
		ts := TaskStatus{
			task.ID,
			task.Status.Timestamp,
			task.DesiredState,
			task.Status.State,
			task.Status.Message,
			task.Status.Err,
			task.Spec.ContainerSpec.Image,
		}

		taskStatus = append(taskStatus, ts)
	}

	return taskStatus
}

func (s *Service) isImageDeploy(swarmTask []swarm.Task, image string) bool {
	imageDeployed := false
	for _, task := range swarmTask {
		if s.getImage(task.Spec.ContainerSpec.Image) == image {
			imageDeployed = true
		}
	}

	return imageDeployed
}

func (s *Service) getImage(image string) string {
	currentImage := strings.Split(image, "@")
	return currentImage[0]
}

func (s *Service) taskStateCount(serviceStatus ServiceStatus, image string) (int, int) {
	runningTaskCount := 0
	errorTaskCount := 0

	for _, ds := range serviceStatus.TaskStatus {

		if (ds.State == swarm.TaskStateFailed || ds.State == swarm.TaskStateRejected) && ds.DesiredState == swarm.TaskStateShutdown && (s.getImage(ds.Image) == image || image == "") {
			errorTaskCount = errorTaskCount + 1
		}

		if ds.State == swarm.TaskStateRunning && ds.DesiredState == swarm.TaskStateRunning && (s.getImage(ds.Image) == image || image == "") {
			runningTaskCount = runningTaskCount + 1
		}
	}

	return runningTaskCount, errorTaskCount
}

func (s *Service) writeConfig() {

}

func (s *Service) reloadProxyServer() {
	outputCmd, err := exec.Command("/usr/local/openresty/bin/openresty", "-s", "reload").Output()
	if err != nil {
		fmt.Printf("Failed to execute command: %s", outputCmd)
		fmt.Println(err)
	}
}

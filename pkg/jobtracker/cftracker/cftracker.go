package cftracker

import (
	"errors"
	"time"

	"github.com/cloudfoundry-community/go-cfclient"
	"github.com/dgruber/drmaa2interface"
	"github.com/dgruber/drmaa2os"
	"github.com/dgruber/drmaa2os/pkg/helper"
	"github.com/dgruber/drmaa2os/pkg/jobtracker"
	fake "github.com/dgruber/drmaa2os/pkg/jobtracker/cftracker/fakes"
)

// clientwrapper interface declares methods of cfclient.Client which
// are used within cftracker. This abstraction seems to be required
// for proper testing.
type clientwrapper interface {
	ListTasks() ([]cfclient.Task, error)
	CreateTask(cfclient.TaskRequest) (cfclient.Task, error)
	TaskByGuid(string) (cfclient.Task, error)
	TerminateTask(string) error
	ListApps() ([]cfclient.App, error)
}

type cftracker struct {
	jobsession string
	config     *cfclient.Config
	client     clientwrapper
}

func newFake(addr, username, password, jobsession string) *cftracker {
	cf := fake.NewClientFake()
	tracker := cftracker{
		jobsession: jobsession,
		config: &cfclient.Config{
			ApiAddress: addr,
			Username:   username,
			Password:   password,
		},
		client: cf,
	}
	return &tracker
}

// init registers the Cloud Foundry tracker at the SessionManager
func init() {
	var a allocator
	drmaa2os.RegisterJobTracker(drmaa2os.CloudFoundrySession, &a)
}

type allocator struct{}

// New is called by the SessionManager when a new JobSession is allocated.
func (a *allocator) New(jobSessionName string, jobTrackerInitParams interface{}) (jobtracker.JobTracker, error) {
	cfParams, ok := jobTrackerInitParams.([]string)
	if !ok {
		return nil, errors.New("jobTrackerInitParams is not of type []string")
	}
	return New(cfParams[0], cfParams[1], cfParams[2], jobSessionName)
}

func New(addr, username, password, jobsession string) (*cftracker, error) {
	config := &cfclient.Config{
		ApiAddress: addr,
		Username:   username,
		Password:   password,
	}
	client, err := cfclient.NewClient(config)
	if err != nil {
		return nil, err
	}
	return &cftracker{
		jobsession: jobsession,
		config:     config,
		client:     client,
	}, nil
}

func (dt *cftracker) ListJobs() ([]string, error) {
	tasks, err := dt.client.ListTasks()
	if err != nil {
		return nil, err
	}
	return convertTasksInNames(tasks), nil
}

func (dt *cftracker) AddJob(jt drmaa2interface.JobTemplate) (string, error) {
	tr, err := convertJobTemplateInTaskRequest(jt)
	if err != nil {
		return "", err
	}
	task, err := dt.client.CreateTask(tr)
	if err != nil {
		return "", err
	}
	return task.GUID, nil
}

func (dt *cftracker) AddArrayJob(jt drmaa2interface.JobTemplate, begin int, end int, step int, maxParallel int) (string, error) {
	if step <= 0 {
		return "", errors.New("step must be greater than 0")
	}
	guids := make([]string, 0, (end-begin)/step)

	var errors error
	for i := begin; i <= end; i += step {
		guid, err := dt.AddJob(jt)
		if err != nil {
			errors = err
			break
		}
		guids = append(guids, guid)
	}
	return helper.Guids2ArrayJobID(guids), errors
}

func (dt *cftracker) ListArrayJobs(ajid string) ([]string, error) {
	return helper.ArrayJobID2GUIDs(ajid)
}

func (dt *cftracker) JobState(jobid string) (drmaa2interface.JobState, string, error) {
	task, err := dt.client.TaskByGuid(jobid)
	if err != nil {
		return drmaa2interface.Undetermined, "", err
	}
	// State of the task. Possible states are PENDING, RUNNING, SUCCEEDED, CANCELING, and FAILED
	switch task.State {
	case "PENDING":
		return drmaa2interface.Queued, "", nil
	case "RUNNING":
		return drmaa2interface.Running, "", nil
	case "CANCELING":
		return drmaa2interface.Running, "", nil
	case "SUCCEEDED":
		return drmaa2interface.Done, "", nil
	case "FAILED":
		return drmaa2interface.Failed, "", nil
	}
	return drmaa2interface.Undetermined, "", nil
}

func (dt *cftracker) JobInfo(jobid string) (drmaa2interface.JobInfo, error) {
	task, err := dt.client.TaskByGuid(jobid)
	if err != nil {
		return drmaa2interface.JobInfo{}, err
	}
	return convertTaskInJobinfo(task), nil
}

func (dt *cftracker) JobControl(jobid, state string) error {
	switch state {
	case "suspend":
		return errors.New("Unsupported Operation")
	case "resume":
		return errors.New("Unsupported Operation")
	case "hold":
		return errors.New("Unsupported Operation")
	case "release":
		return errors.New("Unsupported Operation")
	case "terminate":
		return dt.client.TerminateTask(jobid)
	}
	return errors.New("undefined state")
}

func (dt *cftracker) Wait(jobid string, timeout time.Duration, states ...drmaa2interface.JobState) error {
	return helper.WaitForState(dt, jobid, timeout, states...)
}

func (dt *cftracker) DeleteJob(jobid string) error {
	// purging the task information from cf db
	return errors.New("DeleteJob not implemented")
}

func (dt *cftracker) ListJobCategories() ([]string, error) {
	app, err := dt.client.ListApps()
	if err != nil {
		return nil, err
	}
	appGUIDs := make([]string, 0, len(app))
	for i := range app {
		appGUIDs = append(appGUIDs, app[i].Guid)
	}
	return appGUIDs, nil
}

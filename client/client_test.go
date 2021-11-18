package client

import (
	"fmt"
	"github.com/google/uuid"
	"testing"
)

func TestProjectService_GetProjectList(t *testing.T) {
	client := NewClient("http://localhost", 8080)
	if list, err := client.Project.GetProjectList(); err != nil {
		t.Errorf("Could not get project list: %s", err.Error())
	} else {
		t.Logf("Got %d projects from list", len(list))
		for _, project := range list {
			t.Logf("Got project %s with ID %d", project.Name, project.Id)
		}
	}
}

func TestProjectService_AddProject(t *testing.T) {
	client := NewClient("http://localhost", 8080)
	name, _ := uuid.NewUUID()
	if project, err := client.Project.AddProject(name.String(), ""); err != nil {
		t.Errorf("Could not create project '%s': %s", name, err.Error())
	} else {
		t.Logf("Created project '%s' with ID '%d'", project.Name, project.Id)
		if name.String() != project.Name {
			t.Errorf("Returned name '%s' does not matche expected name of '%s'", project.Name, name)
		}
	}
}

func TestResponseService_GetResponse(t *testing.T) {
	client := NewClient("http://localhost", 8080)
	if responses, err := client.Response.GetResponse("IT", "How are you doing?", ""); err != nil {
		t.Errorf("Failed to get response: %s", err.Error())
	} else {
		t.Logf("Got %d responses", len(responses.Results))
		for _, res := range responses.Results {
			fmt.Printf("Query: '%s' - Response: '%s' - Score: '%.2f'", res.Question, res.Answer, res.Score)
		}
	}
}

package cautem

import (
	"context"
	"encoding/json"
	"fmt"

	datamodelv1 "github.com/NVIDIA/OpenShell/sdk/go/proto/datamodelv1"
	compatv1 "github.com/NVIDIA/OpenShell/sdk/go/proto/openshellv1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

// WorkloadTemplate is the portable, workspace-scoped workload shape exposed by
// the OpenShell template API.
type WorkloadTemplate struct {
	Name             string            `json:"name"`
	Workspace        string            `json:"workspace,omitempty"`
	Image            string            `json:"image"`
	Environment      map[string]string `json:"environment,omitempty"`
	CPU              string            `json:"cpu,omitempty"`
	Memory           string            `json:"memory,omitempty"`
	GPU              bool              `json:"gpu,omitempty"`
	GPUCount         uint32            `json:"gpu_count,omitempty"`
	Labels           map[string]string `json:"labels,omitempty"`
	DriverConfigJSON string            `json:"driver_config_json,omitempty"`
}

// CreateSandboxTemplate creates a template in the selected workspace.
func (c *Client) CreateSandboxTemplate(ctx context.Context, template WorkloadTemplate) (WorkloadTemplate, error) {
	if template.Name == "" || template.Image == "" {
		return WorkloadTemplate{}, fmt.Errorf("create sandbox template: name and image are required")
	}
	workspace := template.Workspace
	if workspace == "" {
		workspace = c.workspace()
	}
	resources := &compatv1.SandboxResources{Cpu: template.CPU, Memory: template.Memory}
	if template.GPU {
		gpu := &compatv1.GpuResourceRequirements{}
		if template.GPUCount > 0 {
			gpu.Count = proto.Uint32(template.GPUCount)
		}
		resources.Gpu = gpu
	}
	var driverConfig *structpb.Struct
	if template.DriverConfigJSON != "" {
		var values map[string]any
		if err := json.Unmarshal([]byte(template.DriverConfigJSON), &values); err != nil || len(values) == 0 {
			return WorkloadTemplate{}, fmt.Errorf("create sandbox template: driver config must be a non-empty JSON object")
		}
		var err error
		driverConfig, err = structpb.NewStruct(values)
		if err != nil {
			return WorkloadTemplate{}, fmt.Errorf("create sandbox template: driver config: %w", err)
		}
	}
	client, err := c.compatClient()
	if err != nil {
		return WorkloadTemplate{}, err
	}
	response, err := client.CreateSandboxTemplate(ctx, &compatv1.CreateSandboxTemplateRequest{
		WorkspaceScope: namedTemplateWorkspace(workspace),
		Template: &compatv1.SandboxWorkloadTemplate{
			Metadata: &datamodelv1.ObjectMeta{Name: template.Name, Workspace: workspace, Labels: cloneStringMap(template.Labels)},
			Spec: &compatv1.SandboxWorkloadTemplateSpec{
				Workload:     &compatv1.SandboxWorkloadConfig{Image: template.Image, Environment: cloneStringMap(template.Environment), Resources: resources},
				DriverConfig: driverConfig,
			},
		},
	})
	if err != nil {
		return WorkloadTemplate{}, fmt.Errorf("create sandbox template %q: %w", template.Name, err)
	}
	return workloadTemplateFromProto(response.GetTemplate()), nil
}

// GetSandboxTemplate returns a template visible in the selected workspace.
func (c *Client) GetSandboxTemplate(ctx context.Context, workspace, name string) (WorkloadTemplate, error) {
	client, err := c.compatClient()
	if err != nil {
		return WorkloadTemplate{}, err
	}
	if workspace == "" {
		workspace = c.workspace()
	}
	response, err := client.GetSandboxTemplate(ctx, &compatv1.GetSandboxTemplateRequest{WorkspaceScope: namedTemplateWorkspace(workspace), Name: name})
	if err != nil {
		return WorkloadTemplate{}, fmt.Errorf("get sandbox template %q: %w", name, err)
	}
	return workloadTemplateFromProto(response.GetTemplate()), nil
}

// ListSandboxTemplates lists templates visible in the selected workspace.
func (c *Client) ListSandboxTemplates(ctx context.Context, workspace string) ([]WorkloadTemplate, error) {
	client, err := c.compatClient()
	if err != nil {
		return nil, err
	}
	if workspace == "" {
		workspace = c.workspace()
	}
	result := make([]WorkloadTemplate, 0)
	pageToken := ""
	seenTokens := map[string]struct{}{}
	for {
		response, err := client.ListSandboxTemplates(ctx, &compatv1.ListSandboxTemplatesRequest{
			WorkspaceScope: namedTemplateWorkspace(workspace), PageSize: 1000, PageToken: pageToken,
		})
		if err != nil {
			return nil, fmt.Errorf("list sandbox templates: %w", err)
		}
		for _, template := range response.GetTemplates() {
			result = append(result, workloadTemplateFromProto(template))
		}
		pageToken = response.GetNextPageToken()
		if pageToken == "" {
			break
		}
		if _, seen := seenTokens[pageToken]; seen {
			return nil, fmt.Errorf("list sandbox templates: gateway repeated a pagination token")
		}
		seenTokens[pageToken] = struct{}{}
	}
	return result, nil
}

// DeleteSandboxTemplate deletes a template in the selected workspace.
func (c *Client) DeleteSandboxTemplate(ctx context.Context, workspace, name string) (bool, error) {
	client, err := c.compatClient()
	if err != nil {
		return false, err
	}
	if workspace == "" {
		workspace = c.workspace()
	}
	response, err := client.DeleteSandboxTemplate(ctx, &compatv1.DeleteSandboxTemplateRequest{WorkspaceScope: namedTemplateWorkspace(workspace), Name: name, AllowMissing: true})
	if err != nil {
		return false, fmt.Errorf("delete sandbox template %q: %w", name, err)
	}
	return response.GetOutcome() == compatv1.DeletionOutcome_DELETION_OUTCOME_COMPLETED, nil
}

func namedTemplateWorkspace(workspace string) *datamodelv1.WorkspaceSelector {
	return &datamodelv1.WorkspaceSelector{Selection: &datamodelv1.WorkspaceSelector_Workspace{Workspace: workspace}}
}

func workloadTemplateFromProto(template *compatv1.SandboxWorkloadTemplate) WorkloadTemplate {
	if template == nil {
		return WorkloadTemplate{}
	}
	result := WorkloadTemplate{Environment: map[string]string{}, Labels: map[string]string{}}
	if metadata := template.GetMetadata(); metadata != nil {
		result.Name, result.Workspace, result.Labels = metadata.GetName(), metadata.GetWorkspace(), cloneStringMap(metadata.GetLabels())
	}
	if spec := template.GetSpec(); spec != nil {
		if config := spec.GetWorkload(); config != nil {
			result.Image, result.Environment = config.GetImage(), cloneStringMap(config.GetEnvironment())
			if resources := config.GetResources(); resources != nil {
				result.CPU, result.Memory = resources.GetCpu(), resources.GetMemory()
				if resources.GetGpu() != nil {
					result.GPU = true
					result.GPUCount = resources.GetGpu().GetCount()
				}
			}
		}
		if config := spec.GetDriverConfig(); config != nil {
			if encoded, err := json.Marshal(config.AsMap()); err == nil {
				result.DriverConfigJSON = string(encoded)
			}
		}
	}
	return result
}

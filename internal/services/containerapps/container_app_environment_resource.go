package containerapps

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/go-azure-helpers/lang/response"
	"github.com/hashicorp/go-azure-helpers/resourcemanager/commonschema"
	"github.com/hashicorp/go-azure-helpers/resourcemanager/location"
	"github.com/hashicorp/go-azure-helpers/resourcemanager/tags"
	"github.com/hashicorp/go-azure-sdk/resource-manager/containerapps/2022-03-01/managedenvironments"
	"github.com/hashicorp/terraform-provider-azurerm/internal/sdk"
	loganalyticsParse "github.com/hashicorp/terraform-provider-azurerm/internal/services/loganalytics/parse"
	loganalyticsValidate "github.com/hashicorp/terraform-provider-azurerm/internal/services/loganalytics/validate"
	networkValidate "github.com/hashicorp/terraform-provider-azurerm/internal/services/network/validate"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tf/pluginsdk"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tf/validation"
	"github.com/hashicorp/terraform-provider-azurerm/utils"
)

type ContainerAppEnvironmentResource struct{}

type ContainerAppEnvironmentModel struct {
	Name                        string                 `tfschema:"name"`
	ResourceGroup               string                 `tfschema:"resource_group_name"`
	Location                    string                 `tfschema:"location"`
	LogAnalyticsWorkspaceId     string                 `tfschema:"log_analytics_workspace_id"`
	InfrastructureSubnetId      string                 `tfschema:"infrastructure_subnet_id"`
	InternalLoadBalancerEnabled bool                   `tfschema:"internal_load_balancer_enabled"`
	Tags                        map[string]interface{} `tfschema:"tags"`

	DefaultDomain         string `tfschema:"default_domain"`
	DockerBridgeCidr      string `tfschema:"docker_bridge_cidr"`
	PlatformReservedCidr  string `tfschema:"platform_reserved_cidr"`
	PlatformReservedDnsIP string `tfschema:"platform_reserved_dns_ip"`
	StaticIP              string `tfschema:"static_ip"`

	// System Data - R/O
	CreatedAt          string `tfschema:"created_at"`
	CreatedBy          string `tfschema:"created_by"`
	CreatedByType      string `tfschema:"created_by_type"`
	LastModifiedAt     string `tfschema:"last_modified_at"`
	LastModifiedBy     string `tfschema:"last_modified_by"`
	LastModifiedByType string `tfschema:"last_modified_by_type"`
}

var _ sdk.ResourceWithUpdate = ContainerAppEnvironmentResource{}

func (r ContainerAppEnvironmentResource) ModelObject() interface{} {
	return &ContainerAppEnvironmentModel{}
}

func (r ContainerAppEnvironmentResource) ResourceType() string {
	return "azurerm_container_app_environment"
}

func (r ContainerAppEnvironmentResource) IDValidationFunc() pluginsdk.SchemaValidateFunc {
	return managedenvironments.ValidateManagedEnvironmentID
}

func (r ContainerAppEnvironmentResource) Arguments() map[string]*pluginsdk.Schema {
	return map[string]*pluginsdk.Schema{
		"name": {
			Type:         pluginsdk.TypeString,
			Required:     true,
			ForceNew:     true,
			ValidateFunc: validation.StringIsNotEmpty, // There are no meaningful indicators for validation here, seems any non-empty string is valid at the Portal?
			Description:  "The name of the Container Apps Managed Environment.",
		},

		"location": commonschema.Location(),

		"resource_group_name": commonschema.ResourceGroupName(),

		"log_analytics_workspace_id": {
			Type:         pluginsdk.TypeString,
			Required:     true,
			ForceNew:     true,
			ValidateFunc: loganalyticsValidate.LogAnalyticsWorkspaceID,
			Description:  "The ID for the Log Analytics Workspace to link this Container Apps Managed Environment to.",
		},

		"infrastructure_subnet_id": {
			Type:         pluginsdk.TypeString,
			Optional:     true,
			Computed:     true,
			ForceNew:     true,
			ValidateFunc: networkValidate.SubnetID,
			Description:  "The existing Subnet to use for the Container Apps Control Plane. **NOTE:** The Subnet must have a `/21` or larger address space.",
		},

		"internal_load_balancer_enabled": {
			Type:        pluginsdk.TypeBool,
			Optional:    true,
			ForceNew:    true,
			Default:     false,
			Description: "Should the Container Environment operate in Internal Load Balancing Mode? Defaults to `false`. **Note:** can only be set to `true` if `infrastructure_subnet_id` is specified.",
		},

		"tags": commonschema.Tags(),
	}
}

func (r ContainerAppEnvironmentResource) Attributes() map[string]*pluginsdk.Schema {
	return map[string]*pluginsdk.Schema{
		"created_at": {
			Type:     pluginsdk.TypeString,
			Computed: true,
		},

		"created_by": {
			Type:     pluginsdk.TypeString,
			Computed: true,
		},

		"created_by_type": {
			Type:     pluginsdk.TypeString,
			Computed: true,
		},

		"default_domain": {
			Type:     pluginsdk.TypeString,
			Computed: true,
		},

		"docker_bridge_cidr": {
			Type:     pluginsdk.TypeString,
			Computed: true,
		},

		"last_modified_at": {
			Type:     pluginsdk.TypeString,
			Computed: true,
		},

		"last_modified_by": {
			Type:     pluginsdk.TypeString,
			Computed: true,
		},

		"last_modified_by_type": {
			Type:     pluginsdk.TypeString,
			Computed: true,
		},

		"platform_reserved_cidr": {
			Type:     pluginsdk.TypeString,
			Computed: true,
		},

		"platform_reserved_dns_ip": {
			Type:     pluginsdk.TypeString,
			Computed: true,
		},

		"static_ip": {
			Type:     pluginsdk.TypeString,
			Computed: true,
		},
	}
}

func (r ContainerAppEnvironmentResource) Create() sdk.ResourceFunc {
	return sdk.ResourceFunc{
		Timeout: 30 * time.Minute,
		Func: func(ctx context.Context, metadata sdk.ResourceMetaData) error {
			client := metadata.Client.ContainerApps.ManagedEnvironmentClient
			logAnalyticsClient := metadata.Client.LogAnalytics.WorkspacesClient
			sharedKeyClient := metadata.Client.LogAnalytics.SharedKeysClient
			subscriptionId := metadata.Client.Account.SubscriptionId

			var containerAppEnvironment ContainerAppEnvironmentModel

			if err := metadata.Decode(&containerAppEnvironment); err != nil {
				return err
			}

			id := managedenvironments.NewManagedEnvironmentID(subscriptionId, containerAppEnvironment.ResourceGroup, containerAppEnvironment.Name)

			existing, err := client.Get(ctx, id)
			if err != nil {
				if !response.WasNotFound(existing.HttpResponse) {
					return fmt.Errorf("checking for presence of existing %s: %+v", id, err)
				}
			}

			if !response.WasNotFound(existing.HttpResponse) {
				return metadata.ResourceRequiresImport(r.ResourceType(), id)
			}

			logAnalyticsId, err := loganalyticsParse.LogAnalyticsWorkspaceID(containerAppEnvironment.LogAnalyticsWorkspaceId)
			if err != nil {
				return err
			}

			workspace, err := logAnalyticsClient.Get(ctx, logAnalyticsId.ResourceGroup, logAnalyticsId.WorkspaceName)
			if err != nil || workspace.WorkspaceProperties == nil {
				return fmt.Errorf("retrieving %s for %s: %+v", logAnalyticsId, id, err)
			}

			if workspace.WorkspaceProperties.CustomerID == nil {
				return fmt.Errorf("reading customer ID from %s", logAnalyticsId)
			}

			keys, err := sharedKeyClient.GetSharedKeys(ctx, logAnalyticsId.ResourceGroup, logAnalyticsId.WorkspaceName)
			if err != nil {
				return fmt.Errorf("retrieving access keys to %s for %s: %+v", logAnalyticsId, id, err)
			}

			if keys.PrimarySharedKey == nil {
				return fmt.Errorf("reading shared key for %s in %s", logAnalyticsId, id)
			}

			managedEnvironment := managedenvironments.ManagedEnvironment{
				Location: containerAppEnvironment.Location,
				Name:     utils.String(containerAppEnvironment.Name),
				Properties: &managedenvironments.ManagedEnvironmentProperties{
					AppLogsConfiguration: &managedenvironments.AppLogsConfiguration{
						Destination: utils.String("log-analytics"),
						LogAnalyticsConfiguration: &managedenvironments.LogAnalyticsConfiguration{
							CustomerId: workspace.WorkspaceProperties.CustomerID,
							SharedKey:  keys.PrimarySharedKey,
						},
					},
					VnetConfiguration: &managedenvironments.VnetConfiguration{},
				},
				Tags: tags.Expand(containerAppEnvironment.Tags),
			}

			if containerAppEnvironment.InfrastructureSubnetId != "" {
				managedEnvironment.Properties.VnetConfiguration.InfrastructureSubnetId = utils.String(containerAppEnvironment.InfrastructureSubnetId)
				managedEnvironment.Properties.VnetConfiguration.Internal = utils.Bool(containerAppEnvironment.InternalLoadBalancerEnabled)
			}

			if err := client.CreateOrUpdateThenPoll(ctx, id, managedEnvironment); err != nil {
				return fmt.Errorf("creating %s: %+v", id, err)
			}

			metadata.SetID(id)
			return nil
		},
	}
}

func (r ContainerAppEnvironmentResource) Read() sdk.ResourceFunc {
	return sdk.ResourceFunc{
		Timeout: 5 * time.Minute,
		Func: func(ctx context.Context, metadata sdk.ResourceMetaData) error {
			client := metadata.Client.ContainerApps.ManagedEnvironmentClient
			id, err := managedenvironments.ParseManagedEnvironmentID(metadata.ResourceData.Id())
			if err != nil {
				return err
			}

			existing, err := client.Get(ctx, *id)
			if err != nil {
				if response.WasNotFound(existing.HttpResponse) {
					return metadata.MarkAsGone(id)
				}
				return fmt.Errorf("reading %s: %+v", *id, err)
			}

			var state ContainerAppEnvironmentModel

			if model := existing.Model; model != nil {
				state.Name = id.EnvironmentName
				state.ResourceGroup = id.ResourceGroupName
				state.Location = location.Normalize(model.Location)
				state.Tags = tags.Flatten(model.Tags)

				if props := model.Properties; props != nil {
					if vnet := props.VnetConfiguration; vnet != nil {
						state.InfrastructureSubnetId = utils.NormalizeNilableString(vnet.InfrastructureSubnetId)
						state.InternalLoadBalancerEnabled = utils.NormaliseNilableBool(vnet.Internal)
						state.DockerBridgeCidr = utils.NormalizeNilableString(vnet.DockerBridgeCidr)
						state.PlatformReservedCidr = utils.NormalizeNilableString(vnet.PlatformReservedCidr)
						state.PlatformReservedDnsIP = utils.NormalizeNilableString(vnet.PlatformReservedDnsIP)
					}

					state.StaticIP = utils.NormalizeNilableString(props.StaticIP)
					state.DefaultDomain = utils.NormalizeNilableString(props.DefaultDomain)
				}

				if sysData := model.SystemData; sysData != nil {
					state.CreatedAt = sysData.CreatedAt
					state.CreatedBy = sysData.CreatedBy
					state.CreatedByType = sysData.CreatedByType
					state.LastModifiedAt = sysData.LastModifiedAt
					state.LastModifiedBy = sysData.LastModifiedBy
					state.LastModifiedByType = sysData.LastModifiedbyType
				}
			}

			// Reading in log_analytics_workspace_id is not possible, so reading from config. Import will need to ignore_changes unfortunately
			if v := metadata.ResourceData.Get("log_analytics_workspace_id").(string); v != "" {
				state.LogAnalyticsWorkspaceId = v
			}

			if err := metadata.Encode(&state); err != nil {
				return fmt.Errorf("encoding: %+v", err)
			}

			return nil
		},
	}
}

func (r ContainerAppEnvironmentResource) Delete() sdk.ResourceFunc {
	return sdk.ResourceFunc{
		Timeout: 30 * time.Minute,
		Func: func(ctx context.Context, metadata sdk.ResourceMetaData) error {
			client := metadata.Client.ContainerApps.ManagedEnvironmentClient
			id, err := managedenvironments.ParseManagedEnvironmentID(metadata.ResourceData.Id())
			if err != nil {
				return err
			}

			if err = client.DeleteThenPoll(ctx, *id); err != nil {
				return fmt.Errorf("deleting %s: %+v", *id, err)
			}

			return nil
		},
	}
}

func (r ContainerAppEnvironmentResource) Update() sdk.ResourceFunc {
	return sdk.ResourceFunc{
		Timeout: 30 * time.Minute,
		Func: func(ctx context.Context, metadata sdk.ResourceMetaData) error {
			client := metadata.Client.ContainerApps.ManagedEnvironmentClient
			id, err := managedenvironments.ParseManagedEnvironmentID(metadata.ResourceData.Id())
			if err != nil {
				return err
			}

			var state ContainerAppEnvironmentModel
			if err := metadata.Decode(&state); err != nil {
				return fmt.Errorf("decoding: %+v", err)
			}

			existing, err := client.Get(ctx, *id)
			if err != nil {
				return fmt.Errorf("reading %s: %+v", *id, err)
			}

			if metadata.ResourceData.HasChange("tags") {
				existing.Model.Tags = tags.Expand(state.Tags)
			}

			if err := client.CreateOrUpdateThenPoll(ctx, *id, *existing.Model); err != nil {
				return fmt.Errorf("updating %s: %+v", id, err)
			}

			return nil
		},
	}
}

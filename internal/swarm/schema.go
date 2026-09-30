// Package swarm models the Docker Swarm service specification of a Dokploy
// service as the `swarm` attribute (v1.8.0, #69). One schema and one
// expand/flatten pair serve dokploy_application and the six database
// resources; compose services have no swarm columns.
//
// The attribute is plain Optional, never Computed, like preview_deployments
// on dokploy_application: an Optional+Computed nested attribute would trip the
// MarkComputedNilsAsUnknown trap described in internal/tfutil. An omitted
// block therefore writes null to all eleven columns.
package swarm

import (
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func optionalString(description string, validators ...validator.String) schema.StringAttribute {
	return schema.StringAttribute{Optional: true, Description: description, Validators: validators}
}

func optionalInt(description string) schema.Int64Attribute {
	return schema.Int64Attribute{
		Optional:    true,
		Description: description,
		Validators:  []validator.Int64{int64validator.AtLeast(0)},
	}
}

func optionalStrings(description string) schema.ListAttribute {
	return schema.ListAttribute{Optional: true, ElementType: types.StringType, Description: description}
}

func optionalObject(description string, attributes map[string]schema.Attribute) schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{Optional: true, Description: description, Attributes: attributes}
}

func optionalObjects(description string, attributes map[string]schema.Attribute) schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Optional:     true,
		Description:  description,
		NestedObject: schema.NestedAttributeObject{Attributes: attributes},
	}
}

// updatePolicy is the shape of update_config and rollback_config. Dokploy
// requires parallelism and order whenever the object is present.
func updatePolicy(action string) map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"parallelism": schema.Int64Attribute{
			Required:    true,
			Description: "Number of tasks that " + action + " at the same time. The value `0` means all tasks at once.",
			Validators:  []validator.Int64{int64validator.AtLeast(0)},
		},
		"delay": optionalInt("Time between two batches, in nanoseconds."),
		"failure_action": optionalString("Action if a task fails: `continue`, `pause`, or `rollback`.",
			stringvalidator.OneOf("continue", "pause", "rollback")),
		"monitor":           optionalInt("Time to watch each task for a failure after it starts, in nanoseconds."),
		"max_failure_ratio": schema.Float64Attribute{Optional: true, Description: "Fraction of failed tasks that the service tolerates, from `0` to `1`."},
		"order": schema.StringAttribute{
			Required:    true,
			Description: "Order of the tasks: `start-first` or `stop-first`.",
			Validators:  []validator.String{stringvalidator.OneOf("start-first", "stop-first")},
		},
	}
}

// Attribute returns the `swarm` attribute. Each nested attribute maps to one
// Dokploy column; the descriptions state the unit of every number.
func Attribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Optional: true,
		Description: "The Docker Swarm service specification. Each attribute maps to one Dokploy column and keeps the Docker field names in snake case. " +
			"Omit the block to write null to every column. A change starts a redeploy when `deploy_on_change` is true.",
		Attributes: map[string]schema.Attribute{
			"health_check": optionalObject("The container health check.", map[string]schema.Attribute{
				"test":         optionalStrings("The check command, for example `[\"CMD\", \"curl\", \"-f\", \"http://localhost\"]`."),
				"interval":     optionalInt("Time between two checks, in nanoseconds."),
				"timeout":      optionalInt("Time after which a check fails, in nanoseconds."),
				"start_period": optionalInt("Time after the start in which a failed check does not count, in nanoseconds."),
				"retries":      optionalInt("Number of failed checks after which the container is unhealthy."),
			}),
			"restart_policy": optionalObject("The restart policy of the tasks.", map[string]schema.Attribute{
				"condition": optionalString("Condition to restart a task: `none`, `on-failure`, or `any`.",
					stringvalidator.OneOf("none", "on-failure", "any")),
				"delay":        optionalInt("Time between two restarts, in nanoseconds."),
				"max_attempts": optionalInt("Number of restarts before Swarm stops the attempts."),
				"window":       optionalInt("Time to wait before Swarm decides that a restart worked, in nanoseconds."),
			}),
			"placement": optionalObject("Where Swarm can place the tasks.", map[string]schema.Attribute{
				"constraints": optionalStrings("Placement constraints, for example `[\"node.labels.tier == app\"]`."),
				"preferences": optionalObjects("Placement preferences.", map[string]schema.Attribute{
					"spread": schema.SingleNestedAttribute{
						Required:    true,
						Description: "Spread the tasks over the values of a node label.",
						Attributes: map[string]schema.Attribute{
							"spread_descriptor": schema.StringAttribute{
								Required:    true,
								Description: "The node label to spread over, for example `node.labels.zone`.",
							},
						},
					},
				}),
				"max_replicas": optionalInt("Maximum number of tasks on one node. The value `0` means no limit."),
				"platforms": optionalObjects("Platforms that Swarm can use.", map[string]schema.Attribute{
					"architecture": schema.StringAttribute{Required: true, Description: "The CPU architecture, for example `amd64`."},
					"os":           schema.StringAttribute{Required: true, Description: "The operating system, for example `linux`."},
				}),
			}),
			"update_config":   optionalObject("The rolling update policy.", updatePolicy("update")),
			"rollback_config": optionalObject("The policy for a rollback after a failed update.", updatePolicy("roll back")),
			"mode": optionalObject("The service mode. If this attribute is set, Dokploy uses it and ignores the top-level `replicas` attribute. "+
				"Set `swarm.mode` or `replicas`, not both: the provider rejects the pair at plan time. "+
				"Swarm cannot change the mode of a service that exists.", map[string]schema.Attribute{
				"replicated": optionalObject("Run a set number of tasks.", map[string]schema.Attribute{
					"replicas": optionalInt("Number of tasks."),
				}),
				"global": optionalObject("Run one task on each node. Set the attribute to `{}`.", map[string]schema.Attribute{}),
				"replicated_job": optionalObject("Run a job with a set number of completions.", map[string]schema.Attribute{
					"max_concurrent":    optionalInt("Maximum number of tasks that run at the same time."),
					"total_completions": optionalInt("Number of tasks that must complete."),
				}),
				"global_job": optionalObject("Run a job once on each node. Set the attribute to `{}`.", map[string]schema.Attribute{}),
			}),
			"labels": schema.MapAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "Docker labels of the service containers.",
			},
			"network": optionalObjects("Swarm networks that the service joins.", map[string]schema.Attribute{
				"target":      schema.StringAttribute{Optional: true, Description: "Name or ID of the network."},
				"aliases":     optionalStrings("Network aliases of the service."),
				"driver_opts": schema.MapAttribute{Optional: true, ElementType: types.StringType, Description: "Driver options of the network attachment."},
			}),
			"endpoint_spec": optionalObject("The endpoint of the service.", map[string]schema.Attribute{
				"mode": optionalString("Endpoint mode: `vip` or `dnsrr`.", stringvalidator.OneOf("vip", "dnsrr")),
				"ports": optionalObjects("Published ports of the service.", map[string]schema.Attribute{
					"protocol":       optionalString("Protocol: `tcp`, `udp`, or `sctp`.", stringvalidator.OneOf("tcp", "udp", "sctp")),
					"target_port":    optionalInt("Port in the container."),
					"published_port": optionalInt("Port on the host or on the routing mesh."),
					"publish_mode":   optionalString("Publish mode: `ingress` or `host`.", stringvalidator.OneOf("ingress", "host")),
				}),
			}),
			"ulimits": optionalObjects("Ulimits of the containers.", map[string]schema.Attribute{
				"name": schema.StringAttribute{Required: true, Description: "Name of the limit, for example `nofile`."},
				"soft": schema.Int64Attribute{Required: true, Description: "The soft limit."},
				"hard": schema.Int64Attribute{Required: true, Description: "The hard limit."},
			}),
			"stop_grace_period": optionalInt("Time that Swarm waits after a stop signal before it kills a container, in nanoseconds."),
		},
	}
}

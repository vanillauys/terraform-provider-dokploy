package acctest

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

// The swarm acceptance fixtures (#69). Every service resource (application,
// the six databases) takes the same `swarm` attribute, so the HCL and the
// expected server columns live here once. A test splices the HCL into the
// resource, then compares the columns that a direct API read returns.

// SwarmFull sets every attribute of the swarm block. The values are valid for
// the Dokploy schema but no real service needs to run with them: use it with
// deploy_on_change = false.
const SwarmFull = `
  swarm = {
    mode = { replicated = { replicas = 2 } }
    placement = {
      constraints  = ["node.role == manager"]
      max_replicas = 4
      preferences  = [{ spread = { spread_descriptor = "node.labels.zone" } }]
      platforms    = [{ architecture = "amd64", os = "linux" }]
    }
    update_config = {
      parallelism       = 1
      delay             = 1000000000
      failure_action    = "pause"
      monitor           = 2000000000
      max_failure_ratio = 0.5
      order             = "start-first"
    }
    rollback_config = { parallelism = 1, order = "stop-first" }
    restart_policy  = { condition = "on-failure", delay = 1000000000, max_attempts = 3, window = 5000000000 }
    health_check    = { test = ["CMD", "true"], interval = 10000000000, timeout = 3000000000, start_period = 1000000000, retries = 2 }
    labels          = { "a69.tier" = "app" }
    network         = [{ target = "a69-net", aliases = ["svc"], driver_opts = { "opt" = "1" } }]
    endpoint_spec = {
      mode  = "vip"
      ports = [{ protocol = "tcp", target_port = 80, published_port = 18080, publish_mode = "host" }]
    }
    ulimits           = [{ name = "nofile", soft = 1024, hard = 2048 }]
    stop_grace_period = 10000000000
  }
`

// SwarmFullColumns is what the server holds after SwarmFull.
var SwarmFullColumns = map[string]string{
	"healthCheckSwarm":     `{"Test":["CMD","true"],"Interval":10000000000,"Timeout":3000000000,"StartPeriod":1000000000,"Retries":2}`,
	"restartPolicySwarm":   `{"Condition":"on-failure","Delay":1000000000,"MaxAttempts":3,"Window":5000000000}`,
	"placementSwarm":       `{"Constraints":["node.role == manager"],"MaxReplicas":4,"Preferences":[{"Spread":{"SpreadDescriptor":"node.labels.zone"}}],"Platforms":[{"Architecture":"amd64","OS":"linux"}]}`,
	"updateConfigSwarm":    `{"Parallelism":1,"Delay":1000000000,"FailureAction":"pause","Monitor":2000000000,"MaxFailureRatio":0.5,"Order":"start-first"}`,
	"rollbackConfigSwarm":  `{"Parallelism":1,"Order":"stop-first"}`,
	"modeSwarm":            `{"Replicated":{"Replicas":2}}`,
	"labelsSwarm":          `{"a69.tier":"app"}`,
	"networkSwarm":         `[{"Target":"a69-net","Aliases":["svc"],"DriverOpts":{"opt":"1"}}]`,
	"endpointSpecSwarm":    `{"Mode":"vip","Ports":[{"Protocol":"tcp","TargetPort":80,"PublishedPort":18080,"PublishMode":"host"}]}`,
	"ulimitsSwarm":         `[{"Name":"nofile","Soft":1024,"Hard":2048}]`,
	"stopGracePeriodSwarm": `10000000000`,
}

// SwarmChanged keeps some of the attributes of SwarmFull with new values and
// drops the others, which must then read back null.
const SwarmChanged = `
  swarm = {
    mode = { replicated = { replicas = 3 } }
    update_config = {
      parallelism = 2
      order       = "stop-first"
    }
    labels            = { "a69.tier" = "db", "a69.extra" = "1" }
    stop_grace_period = 20000000000
  }
`

// SwarmChangedColumns is what the server holds after SwarmChanged. An empty
// value means a null column.
var SwarmChangedColumns = map[string]string{
	"modeSwarm":            `{"Replicated":{"Replicas":3}}`,
	"updateConfigSwarm":    `{"Parallelism":2,"Order":"stop-first"}`,
	"labelsSwarm":          `{"a69.tier":"db","a69.extra":"1"}`,
	"stopGracePeriodSwarm": `20000000000`,
}

// SwarmRunnable holds only the attributes that a real service accepts on a
// single-node rig, so a deploy converges with them.
const SwarmRunnable = `
  swarm = {
    mode = { replicated = { replicas = 2 } }
    placement = {
      constraints = ["node.role == manager"]
    }
    update_config = {
      parallelism = 1
      order       = "start-first"
    }
    restart_policy    = { condition = "any", max_attempts = 5 }
    endpoint_spec     = { mode = "vip" }
    labels            = { "a69.tier" = "app" }
    ulimits           = [{ name = "nofile", soft = 1024, hard = 2048 }]
    stop_grace_period = 10000000000
  }
`

// SwarmRunnableColumns is what the server holds after SwarmRunnable.
var SwarmRunnableColumns = map[string]string{
	"modeSwarm":            `{"Replicated":{"Replicas":2}}`,
	"placementSwarm":       `{"Constraints":["node.role == manager"]}`,
	"updateConfigSwarm":    `{"Parallelism":1,"Order":"start-first"}`,
	"restartPolicySwarm":   `{"Condition":"any","MaxAttempts":5}`,
	"endpointSpecSwarm":    `{"Mode":"vip"}`,
	"labelsSwarm":          `{"a69.tier":"app"}`,
	"ulimitsSwarm":         `[{"Name":"nofile","Soft":1024,"Hard":2048}]`,
	"stopGracePeriodSwarm": `10000000000`,
}

// CheckSwarmColumns compares the eleven columns of a server record with the
// expected JSON of each one. A column that want does not list must be null.
func CheckSwarmColumns(got client.Swarm, want map[string]string) error {
	body, err := json.Marshal(got)
	if err != nil {
		return err
	}
	var have map[string]any
	if err := json.Unmarshal(body, &have); err != nil {
		return err
	}
	for column, value := range have {
		var expected any
		if w := want[column]; w != "" {
			if err := json.Unmarshal([]byte(w), &expected); err != nil {
				return fmt.Errorf("fixture %s: %w", column, err)
			}
		}
		if !reflect.DeepEqual(value, expected) {
			return fmt.Errorf("server %s = %v, want %v", column, value, expected)
		}
	}
	return nil
}

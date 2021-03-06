// Copyright © 2017 Aqua Security Software Ltd. <info@aquasec.com>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package check

import (
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v2"
	"strconv"
)

// Controls holds all controls to check for master nodes.
type Controls struct {
	ID      		string   `yaml:"id" json:"id"`
	Version 		string   `json:"version"`
	Text    		string   `json:"text"`
	Type    		NodeType `json:"node_type"`
	UserCISLevel 	string 	 `json:"cis_level"`
	Groups  		[]*Group `json:"tests"`
	Summary
	// Map level -> Summary
	SummaryLevelWise map[string]*Summary
}

// Group is a collection of similar checks.
type Group struct {
	ID     string   `yaml:"id" json:"section"`
	Pass   int      `json:"pass"`
	Fail   int      `json:"fail"`
	Warn   int      `json:"warn"`
	Skip   int      `json:"skip"`
	Info   int      `json:"info"`
	Text   string   `json:"desc"`
	Checks []*Check `json:"results"`
}

// Summary is a summary of the results of control checks run.
type Summary struct {
	Pass int `json:"total_pass"`
	Fail int `json:"total_fail"`
	Warn int `json:"total_warn"`
	Info int `json:"total_info"`
	Skip int `json:"total_skip"`
}

// NewControls instantiates a new master Controls object.
func NewControls(t NodeType, level string, in []byte) (*Controls, error) {
	c := new(Controls)

	c.UserCISLevel = level
	err := yaml.Unmarshal(in, c)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML: %s", err)
	}

	if t != c.Type {
		return nil, fmt.Errorf("non-%s controls file specified", t)
	}

	// Prepare audit commands
	for _, group := range c.Groups {
		for _, check := range group.Checks {
			check.Commands = textToCommand(check.Audit)
		}
	}

	return c, nil
}

// RunGroup runs all checks in a group.
func (controls *Controls) RunGroup(gids ...string) (Summary, error) {
	g := []*Group{}
	controls.SummaryLevelWise = map[string]*Summary{}
	controls.Summary.Pass, controls.Summary.Fail, controls.Summary.Warn, controls.Summary.Skip, controls.Summary.Info = 0, 0, 0, 0, 0
	controls.SummaryLevelWise["1"] = &Summary{ 0,0,0,0,0}
	controls.SummaryLevelWise["2"] = &Summary{ 0,0,0,0,0}

	// If no groupid is passed run all group checks.
	if len(gids) == 0 {
		gids = controls.getAllGroupIDs()
	}

	userCISLevel, err := strconv.ParseUint(controls.UserCISLevel, 10, 64)
	if err != nil{
		return controls.Summary, fmt.Errorf("%s", "error in parsing User CIS level")
	}

	for _, group := range controls.Groups {

		for _, gid := range gids {
			if gid == group.ID {
				for _, check := range group.Checks {
					checkCIS, err := strconv.ParseUint(check.CheckCISLevel, 10, 64)
					if err != nil{
						return controls.Summary, fmt.Errorf("%s", "error in parsing Check CIS level")
					}
					if userCISLevel < checkCIS{
						check.State = SKIP
					}
					check.Run()
					check.TestInfo = append(check.TestInfo, check.Remediation)
					summarize(controls, check)
					summarizeGroup(group, check)
					summarizeLevel(controls, check)
				}

				g = append(g, group)
			}
		}
	}

	controls.Groups = g
	return controls.Summary, nil
}

// RunChecks runs the checks with the supplied IDs.
func (controls *Controls) RunChecks(ids ...string) (Summary, error) {
	g := []*Group{}
	m := make(map[string]*Group)
	controls.SummaryLevelWise = map[string]*Summary{}
	controls.Summary.Pass, controls.Summary.Fail, controls.Summary.Warn, controls.Summary.Skip, controls.Summary.Info = 0, 0, 0, 0, 0
	controls.SummaryLevelWise["1"] = &Summary{ 0,0,0,0,0}
	controls.SummaryLevelWise["2"] = &Summary{ 0,0,0,0,0}

	// If no groupid is passed run all group checks.
	if len(ids) == 0 {
		ids = controls.getAllCheckIDs()
	}

	for _, group := range controls.Groups {
		for _, check := range group.Checks {
			for _, id := range ids {
				if id == check.ID {
					check.Run()
					check.TestInfo = append(check.TestInfo, check.Remediation)
					summarize(controls, check)
					summarizeLevel(controls, check)

					// Check if we have already added this checks group.
					if v, ok := m[group.ID]; !ok {
						// Create a group with same info
						w := &Group{
							ID:     group.ID,
							Text:   group.Text,
							Checks: []*Check{},
						}

						// Add this check to the new group
						w.Checks = append(w.Checks, check)

						// Add to groups we have visited.
						m[w.ID] = w
						g = append(g, w)
					} else {
						v.Checks = append(v.Checks, check)
					}

				}
			}
		}
	}

	controls.Groups = g
	return controls.Summary, nil
}

// JSON encodes the results of last run to JSON.
func (controls *Controls) JSON() ([]byte, error) {
	return json.Marshal(controls)
}

func (controls *Controls) getAllGroupIDs() []string {
	var ids []string

	for _, group := range controls.Groups {
		ids = append(ids, group.ID)
	}
	return ids
}

func (controls *Controls) getAllCheckIDs() []string {
	var ids []string

	for _, group := range controls.Groups {
		for _, check := range group.Checks {
			ids = append(ids, check.ID)
		}
	}
	return ids

}

func summarize(controls *Controls, check *Check) {
	switch check.State {
	case PASS:
		controls.Summary.Pass++
	case FAIL:
		controls.Summary.Fail++
	case WARN:
		controls.Summary.Warn++
	case INFO:
		controls.Summary.Info++
	case SKIP:
		controls.Summary.Skip++
	}
}

func summarizeGroup(group *Group, check *Check) {
	switch check.State {
	case PASS:
		group.Pass++
	case FAIL:
		group.Fail++
	case WARN:
		group.Warn++
	case INFO:
		group.Info++
	case SKIP:
		group.Skip++
	}
}

func summarizeLevel(control *Controls, check *Check) {

	switch check.State{
	case PASS:
		control.SummaryLevelWise[check.CheckCISLevel].Pass++
	case FAIL:
		control.SummaryLevelWise[check.CheckCISLevel].Fail++
	case WARN:
		control.SummaryLevelWise[check.CheckCISLevel].Warn++
	case INFO:
		control.SummaryLevelWise[check.CheckCISLevel].Info++
	case SKIP:
		control.SummaryLevelWise[check.CheckCISLevel].Skip++
	}

}
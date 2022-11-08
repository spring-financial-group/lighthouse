/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package holdtill contains a plugin which will allow users to label their
// own pull requests as not ready or ready for merge. The submit queue
// will honor the label to ensure pull requests do not merge when it is
// applied.
package holdtill

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"regexp"
	"time"

	"github.com/jenkins-x/go-scm/scm"
	"github.com/jenkins-x/lighthouse/pkg/labels"
	"github.com/jenkins-x/lighthouse/pkg/plugins"
	"github.com/jenkins-x/lighthouse/pkg/scmprovider"
)

const (
	pluginName = "holdtill"
	rfc3339    = `((?:(\d{4}-\d{2}-\d{2})T(\d{2}:\d{2}:\d{2}(?:\.\d+)?))(Z|[\+-]\d{2}:\d{2})?)`
)

var (
	rfc3339OrCancel = fmt.Sprintf(`^(%s|cancel)$`, rfc3339)
	labelRegex      = regexp.MustCompile(labels.HoldTill + " " + rfc3339)
)

type hasLabelFunc func(regex *regexp.Regexp, issueLabels []*scm.Label) bool

var (
	plugin = plugins.Plugin{
		Description: "The hold plugin allows anyone to add or remove the '" + labels.HoldTill + "' Label from a pull request in order to prevent the PR from merging till the merge time has passed.",
		Commands: []plugins.Command{{
			Name:        "holdtill",
			Description: "Adds or removes the `" + labels.HoldTill + "` Label which is used to indicate that the PR should not be automatically merged.",
			Arg: &plugins.CommandArg{
				Pattern:  rfc3339OrCancel,
				Optional: false,
			},
			Action: plugins.
				Invoke(func(match plugins.CommandMatch, pc plugins.Agent, e scmprovider.GenericCommentEvent) error {
					return handleGenericComment(match.Arg, pc, e)
				}).
				When(plugins.Action(scm.ActionCreate)),
		}},
	}
)

func init() {
	plugins.RegisterPlugin(pluginName, plugin)
}

type scmProviderClient interface {
	AddLabel(owner, repo string, number int, label string, pr bool) error
	RemoveLabel(owner, repo string, number int, label string, pr bool) error
	GetIssueLabels(org, repo string, number int, pr bool) ([]*scm.Label, error)
}

func handleGenericComment(arg string, pc plugins.Agent, e scmprovider.GenericCommentEvent) error {
	label, err := getLabel(labelRegex, pc.SCMProviderClient, &e)
	if err != nil {
		return err
	}
	if arg == "cancel" {
		if label == nil {
			pc.Logger.Infof("Could not cancel, no %s label found on %s/%s#%d", labels.HoldTill, e.Repo.Namespace, e.Repo.Name, e.Number)
			return nil
		}
		pc.Logger.Infof("Removing %q Label for %s/%s#%d", label.Name, e.Repo.Namespace, e.Repo.Name, e.Number)
		return pc.SCMProviderClient.RemoveLabel(e.Repo.Namespace, e.Repo.Name, e.Number, label.Name, e.IsPR)
	} else if label != nil {
		pc.Logger.Infof("Could not add, %s label already exists on %s/%s#%d", label.Name, e.Repo.Namespace, e.Repo.Name, e.Number)
		return nil
	}

	holdTillTime, err := time.Parse(time.RFC3339, arg)
	if err != nil {
		return err
	}

	return createNewLabel(holdTillTime, pc.SCMProviderClient, pc.Logger, &e)
}

func createNewLabel(holdTillTime time.Time, spc scmProviderClient, log *logrus.Entry, e *scmprovider.GenericCommentEvent) error {

}

func getLabel(regex *regexp.Regexp, spc scmProviderClient, e *scmprovider.GenericCommentEvent) (*scm.Label, error) {
	issueLabels, err := spc.GetIssueLabels(e.Repo.Namespace, e.Repo.Name, e.Number, e.IsPR)
	if err != nil {
		return nil, fmt.Errorf("failed to get the labels on %s/%s#%d: %v", e.Repo.Namespace, e.Repo.Name, e.Number, err)
	}

	for _, l := range issueLabels {
		if regex.MatchString(l.Name) {
			return l, nil
		}
	}
	return nil, nil
}

// handle drives the pull request to the desired state. If any user adds
// a /hold directive, we want to add a label if one does not already exist.
// If they add /hold cancel, we want to remove the label if it exists.
func handle(label string, spc scmProviderClient, log *logrus.Entry, e *scmprovider.GenericCommentEvent, f hasLabelFunc) error {
	needsLabel := !(label == "cancel")

	issueLabels, err := spc.GetIssueLabels(e.Repo.Namespace, e.Repo.Name, e.Number, e.IsPR)
	if err != nil {
		return fmt.Errorf("failed to get the labels on %s/%s#%d: %v", e.Repo.Namespace, e.Repo.Name, e.Number, err)
	}

	hasLabel := f(holdTillRegex, issueLabels)
	if hasLabel && !needsLabel {
		log.Infof("Removing %q Label for %s/%s#%d", labels.Hold, e.Repo.Namespace, e.Repo.Name, e.Number)
		return spc.RemoveLabel(e.Repo.Namespace, e.Repo.Name, e.Number, labels.Hold, e.IsPR)
	} else if !hasLabel && needsLabel {
		log.Infof("Adding %q Label for %s/%s#%d", labels.Hold, e.Repo.Namespace, e.Repo.Name, e.Number)
		return spc.AddLabel(e.Repo.Namespace, e.Repo.Name, e.Number, labels.Hold, e.IsPR)
	}
	return nil
}

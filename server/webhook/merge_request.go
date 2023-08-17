package webhook

import (
	"context"
	"fmt"

	"github.com/xanzy/go-gitlab"
)

func (w *webhook) HandleMergeRequest(ctx context.Context, event *gitlab.MergeEvent) ([]*HandleWebhook, error) {
	handlers, err := w.handleDMMergeRequest(event)
	if err != nil {
		return nil, err
	}
	handlers2, err := w.handleChannelMergeRequest(ctx, event)
	if err != nil {
		return nil, err
	}
	return cleanWebhookHandlers(append(handlers, handlers2...)), nil
}

func (w *webhook) handleDMMergeRequest(event *gitlab.MergeEvent) ([]*HandleWebhook, error) {
	authorGitlabUsername := w.gitlabRetreiver.GetUsernameByID(event.ObjectAttributes.AuthorID)
	senderGitlabUsername := event.User.Username

	message := ""

	switch event.ObjectAttributes.State {
	case stateOpened:
		switch event.ObjectAttributes.Action {
		case actionOpen:
			message = fmt.Sprintf("[%s](%s) requested your review on [%s!%v](%s)", senderGitlabUsername, w.gitlabRetreiver.GetUserURL(senderGitlabUsername), event.ObjectAttributes.Target.PathWithNamespace, event.ObjectAttributes.IID, event.ObjectAttributes.URL)
		case actionReopen:
			message = fmt.Sprintf("[%s](%s) reopen your merge request [%s!%v](%s)", senderGitlabUsername, w.gitlabRetreiver.GetUserURL(senderGitlabUsername), event.ObjectAttributes.Target.PathWithNamespace, event.ObjectAttributes.IID, event.ObjectAttributes.URL)
		case actionUpdate:
			if event.ObjectAttributes.OldRev != "" {
				message = fmt.Sprintf("[%s](%s) pushed a new commit to merge request [%s!%v](%s)", senderGitlabUsername, w.gitlabRetreiver.GetUserURL(senderGitlabUsername), event.ObjectAttributes.Target.PathWithNamespace, event.ObjectAttributes.IID, event.ObjectAttributes.URL)
			} else {
				prevAssignees := event.Changes.Assignees.Previous
				currAssignees := event.Changes.Assignees.Current
				if len(prevAssignees) != len(currAssignees) || !compareAssignees(prevAssignees, currAssignees) {
					message = fmt.Sprintf("[%s](%s) updated the assignees of merge request [%s!%v](%s)", senderGitlabUsername, w.gitlabRetreiver.GetUserURL(senderGitlabUsername), event.ObjectAttributes.Target.PathWithNamespace, event.ObjectAttributes.IID, event.ObjectAttributes.URL)
				}
			}
		case actionApproved:
			message = fmt.Sprintf("[%s](%s) approved your merge request [%s!%v](%s)", senderGitlabUsername, w.gitlabRetreiver.GetUserURL(senderGitlabUsername), event.ObjectAttributes.Target.PathWithNamespace, event.ObjectAttributes.IID, event.ObjectAttributes.URL)
		case actionUnapproved:
			message = fmt.Sprintf("[%s](%s) requested changes to your merge request [%s!%v](%s)", senderGitlabUsername, w.gitlabRetreiver.GetUserURL(senderGitlabUsername), event.ObjectAttributes.Target.PathWithNamespace, event.ObjectAttributes.IID, event.ObjectAttributes.URL)
		}
	case stateClosed:
		message = fmt.Sprintf("[%s](%s) closed your merge request [%s!%v](%s)", senderGitlabUsername, w.gitlabRetreiver.GetUserURL(senderGitlabUsername), event.ObjectAttributes.Target.PathWithNamespace, event.ObjectAttributes.IID, event.ObjectAttributes.URL)
	case stateMerged:
		if event.ObjectAttributes.Action == actionMerge {
			message = fmt.Sprintf("[%s](%s) merged your merge request [%s!%v](%s)", senderGitlabUsername, w.gitlabRetreiver.GetUserURL(senderGitlabUsername), event.ObjectAttributes.Target.PathWithNamespace, event.ObjectAttributes.IID, event.ObjectAttributes.URL)
		}
	}

	if len(message) > 0 {
		handlers := []*HandleWebhook{{
			Message:    message,
			ToUsers:    []string{w.gitlabRetreiver.GetUsernameByID(event.ObjectAttributes.AssigneeID), authorGitlabUsername},
			ToChannels: []string{},
			From:       senderGitlabUsername,
		}}

		if mention := w.handleMention(mentionDetails{
			senderUsername:    senderGitlabUsername,
			pathWithNamespace: event.Project.PathWithNamespace,
			IID:               fmt.Sprintf("%d", event.ObjectAttributes.IID),
			URL:               event.ObjectAttributes.URL,
			body:              sanitizeDescription(event.ObjectAttributes.Description),
		}); mention != nil {
			handlers = append(handlers, mention)
		}
		return handlers, nil
	}
	return []*HandleWebhook{{From: senderGitlabUsername}}, nil
}

func (w *webhook) handleChannelMergeRequest(ctx context.Context, event *gitlab.MergeEvent) ([]*HandleWebhook, error) {
	senderGitlabUsername := event.User.Username
	pr := event.ObjectAttributes
	repo := event.Project
	res := []*HandleWebhook{}

	message := ""

	switch pr.Action {
	case actionOpen:
		message = fmt.Sprintf("#### %s\n##### [%s!%v](%s) new merge-request by [%s](%s) on [%s](%s)\n\n%s", pr.Title, repo.PathWithNamespace, pr.IID, pr.URL, senderGitlabUsername, w.gitlabRetreiver.GetUserURL(senderGitlabUsername), pr.CreatedAt, pr.URL, sanitizeDescription(pr.Description))
	case actionMerge:
		message = fmt.Sprintf("[%s](%s) Merge request [!%v %s](%s) was merged by [%s](%s)", repo.PathWithNamespace, repo.WebURL, pr.IID, pr.Title, pr.URL, senderGitlabUsername, w.gitlabRetreiver.GetUserURL(senderGitlabUsername))
	case actionClose:
		message = fmt.Sprintf("[%s](%s) Merge request [!%v %s](%s) was closed by [%s](%s)", repo.PathWithNamespace, repo.WebURL, pr.IID, pr.Title, pr.URL, senderGitlabUsername, w.gitlabRetreiver.GetUserURL(senderGitlabUsername))
	case actionReopen:
		message = fmt.Sprintf("[%s](%s) Merge request [!%v %s](%s) was reopened by [%s](%s)", repo.PathWithNamespace, repo.WebURL, pr.IID, pr.Title, pr.URL, senderGitlabUsername, w.gitlabRetreiver.GetUserURL(senderGitlabUsername))
	case actionApproved:
		message = fmt.Sprintf("[%s](%s) Merge request [!%v %s](%s) was approved by [%s](%s)", repo.PathWithNamespace, repo.WebURL, pr.IID, pr.Title, pr.URL, senderGitlabUsername, w.gitlabRetreiver.GetUserURL(senderGitlabUsername))
	case actionUnapproved:
		message = fmt.Sprintf("[%s](%s) Merge request [!%v %s](%s) changes were requested by [%s](%s)", repo.PathWithNamespace, repo.WebURL, pr.IID, pr.Title, pr.URL, senderGitlabUsername, w.gitlabRetreiver.GetUserURL(senderGitlabUsername))
	}

	if len(message) > 0 {
		toChannels := make([]string, 0)
		namespace, project := normalizeNamespacedProject(repo.PathWithNamespace)
		subs := w.gitlabRetreiver.GetSubscribedChannelsForProject(
			ctx, namespace, project,
			repo.Visibility == gitlab.PublicVisibility,
		)
		for _, sub := range subs {
			if !sub.Merges() {
				continue
			}

			if sub.Label() != "" && !containsLabelPointer(event.Labels, sub.Label()) {
				continue
			}

			toChannels = append(toChannels, sub.ChannelID)
		}

		if len(toChannels) > 0 {
			res = append(res, &HandleWebhook{
				From:       senderGitlabUsername,
				Message:    message,
				ToUsers:    []string{},
				ToChannels: toChannels,
			})
		}
	}

	return res, nil
}

func compareAssignees(prev, curr []*gitlab.EventUser) bool {
	m := make(map[int]bool)
	for _, assignee := range prev {
		m[assignee.ID] = true
	}
	for _, assignee := range curr {
		if !m[assignee.ID] {
			return false
		}
	}
	return true
}

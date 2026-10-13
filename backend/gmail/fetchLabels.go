package gmail

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"

	gmailapi "google.golang.org/api/gmail/v1"

	"github.com/sarielhp/mail_cli/cache/msgstore"
	"github.com/sarielhp/mail_cli/report"
)

// fetchGmailLabelsREST fetches all Gmail labels with populated message counts,
// split into system and user label lists.
func fetchGmailLabelsREST(rep report.Reporter, config *Config) (systemLabels, userLabels []*gmailapi.Label, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), msgstore.DefaultFetchTimeout)
	defer cancel()

	srv, err := GetGmailServiceContext(ctx, rep, config)
	if err != nil {
		return nil, nil, err
	}

	labelsRes, err := executeGmailOpWithRetry(ctx, "list labels", func() (*gmailapi.ListLabelsResponse, error) {
		return srv.Users.Labels.List("me").Context(ctx).Do()
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list Gmail labels: %w", err)
	}

	for _, l := range labelsRes.Labels {
		if strings.EqualFold(l.Type, "system") {
			systemLabels = append(systemLabels, l)
		} else {
			userLabels = append(userLabels, l)
		}
	}

	allLabels := append(append([]*gmailapi.Label(nil), systemLabels...), userLabels...)
	fetchLabelDetailsConcurrently(ctx, srv, allLabels)
	backfillZeroCountLabels(ctx, srv, userLabels)

	sortLabelsAlphabetically(systemLabels)
	sortLabelsAlphabetically(userLabels)

	return systemLabels, userLabels, nil
}

func fetchLabelDetailsConcurrently(ctx context.Context, srv *gmailapi.Service, list []*gmailapi.Label) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 15)
	var mu sync.Mutex

	for _, l := range list {
		wg.Add(1)
		go func(label *gmailapi.Label) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					slog.Error("panic fetching label detail", slog.Any("recover", r), slog.String("label", label.Name))
				}
			}()
			if ctx.Err() != nil {
				return
			}
			sem <- struct{}{}
			defer func() { <-sem }()

			detail, err := executeLabelGetWithRetry(ctx, srv, label.Id)
			if err != nil {
				slog.Warn("failed to fetch label detail", slog.String("label", label.Name), slog.Any("error", err))
				return
			}
			mu.Lock()
			label.MessagesTotal = detail.MessagesTotal
			label.MessagesUnread = detail.MessagesUnread
			mu.Unlock()
		}(l)
	}
	wg.Wait()
}

func backfillZeroCountLabels(ctx context.Context, srv *gmailapi.Service, userLabels []*gmailapi.Label) {
	var searchWg sync.WaitGroup
	searchSem := make(chan struct{}, 15)
	var mu sync.Mutex

	for _, l := range userLabels {
		if l.MessagesTotal != 0 {
			continue
		}
		searchWg.Add(1)
		go func(label *gmailapi.Label) {
			defer searchWg.Done()
			defer func() {
				if r := recover(); r != nil {
					slog.Error("panic querying label count", slog.Any("recover", r), slog.String("label", label.Name))
				}
			}()
			if ctx.Err() != nil {
				return
			}
			searchSem <- struct{}{}
			defer func() { <-searchSem }()

			updateLabelMessageCounts(ctx, srv, label, &mu)
		}(l)
	}
	searchWg.Wait()
}

func updateLabelMessageCounts(ctx context.Context, srv *gmailapi.Service, label *gmailapi.Label, mu *sync.Mutex) {
	counts := []struct {
		query string
		warn  string
		set   func(*gmailapi.Label, int64)
	}{
		{fmt.Sprintf("label:%q", label.Name), "failed to query total message count for label", func(l *gmailapi.Label, n int64) { l.MessagesTotal = n }},
		{fmt.Sprintf("label:%q is:unread", label.Name), "failed to query unread message count for label", func(l *gmailapi.Label, n int64) { l.MessagesUnread = n }},
	}
	for _, c := range counts {
		if ctx.Err() != nil {
			return
		}
		n, err := labelMessageCount(ctx, srv, c.query)
		if err != nil {
			slog.Warn(c.warn, slog.String("label", label.Name), slog.Any("error", err))
			continue
		}
		mu.Lock()
		c.set(label, n)
		mu.Unlock()
	}
}

// labelMessageCount runs a message-list query and returns how many messages the
// server reports for it, preferring its own estimate when a page token shows
// there are more than the page returned.
func labelMessageCount(ctx context.Context, srv *gmailapi.Service, query string) (int64, error) {
	call := srv.Users.Messages.List("me").Q(query).IncludeSpamTrash(true).Context(ctx)
	res, err := executeListMessagesWithRetry(ctx, call)
	if err != nil {
		return 0, err
	}
	n := int64(len(res.Messages))
	if res.NextPageToken != "" && res.ResultSizeEstimate > n {
		n = res.ResultSizeEstimate
	}
	return n, nil
}

func sortLabelsAlphabetically(list []*gmailapi.Label) {
	sort.Slice(list, func(i, j int) bool {
		return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name)
	})
}

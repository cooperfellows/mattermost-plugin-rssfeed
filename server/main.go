package main

import (
	"github.com/mattermost/mattermost-server/plugin"
)

func main() {
	plugin.ClientMain(&RSSFeedPlugin{
		FeedHandler: newFeedHandler(),
	})
}

package main

import (
	"github.com/lunny/html2md"
	"github.com/mattermost/mattermost-server/mlog"
	"github.com/mattermost/mattermost-server/model"
	"github.com/mattermost/mattermost-server/plugin"
	"io/ioutil"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const RSSFEED_ICON_URL = "https://mattermost.gridprotectionalliance.org/plugins/rssfeed/images/rss.png"
const RSSFEED_USERNAME = "RSSFeed Plugin"

// RSSFeedPlugin Object
type RSSFeedPlugin struct {
	plugin.MattermostPlugin

	// configurationLock synchronizes access to the configuration.
	configurationLock sync.RWMutex

	// configuration is the active plugin configuration. Consult getConfiguration and
	// setConfiguration for usage.
	configuration *configuration
}

// ServeHTTP hook from mattermost plugin
func (p *RSSFeedPlugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	switch path := r.URL.Path; path {
	case "/images/rss.png":
		data, err := ioutil.ReadFile(string("plugins/rssfeed/server/dist/images/rss.png"))
		if err == nil {
			w.Header().Set("Content-Type", "image/png")
			w.Write(data)
		} else {
			w.WriteHeader(404)
			w.Write([]byte("404 Something went wrong - " + http.StatusText(404)))
			p.API.LogInfo("/imags/rss.png err = ", err.Error())
		}
	default:
		w.Header().Set("Content-Type", "application/json")
		http.NotFound(w, r)
	}
}

// OnActivate is a plugin hook from the Mattermost plugin API
func (p *RSSFeedPlugin) OnActivate() error {
	p.API.RegisterCommand(getCommand())
	go p.setupHeartBeat()
	return nil
}

func (p *RSSFeedPlugin) setupHeartBeat() {
	heartbeatTime, err := p.getHeartbeatTime()
	if err != nil {
		p.API.LogError(err.Error())
	}
	p.API.LogInfo("Heartbeat time = " + string(heartbeatTime))
	for true {
		p.API.LogInfo("Heartbeat")

		err := p.processHeartBeat()
		if err != nil {
			p.API.LogError(err.Error())

		}
		time.Sleep(15 * time.Minute)
	}
}

func (p *RSSFeedPlugin) processHeartBeat() error {
	dictionaryOfSubscriptions, err := p.getSubscriptions()
	if err != nil {
		mlog.Error(err.Error())
		return nil
	}

	for _, value := range dictionaryOfSubscriptions.Subscriptions {
		p.processSubscription(value)
	}

	return nil
}

func (p *RSSFeedPlugin) getHeartbeatTime() (int, error) {
	config := p.getConfiguration()
	heartbeatTime := 15
	var err error
	if len(config.Heartbeat) > 0 {
		heartbeatTime, err = strconv.Atoi(config.Heartbeat)
		if err != nil {
			mlog.Error(err.Error())
			return 15, err
		}

	}

	return heartbeatTime, nil
}

func (p *RSSFeedPlugin) processSubscription(subscription *Subscription) error {
	if len(subscription.URL) == 0 {
		return nil
	}

	//p.API.LogInfo("Process subscription. url = " + subscription.URL)
	//p.API.LogInfo("Process subscription. xml = " + subscription.XML)

	// retrieve old xml feed from database
	if len(subscription.XML) > 0 {
		oldRssFeed, err := RssParseString(subscription.XML)
		if err != nil {
			p.API.LogError("Go Feed failed to parse old subscription.")
			p.API.LogError(err.Error())

			return err
		}
		//p.API.LogInfo(fmt.Sprintf("%v", oldRssFeed))

		newRssFeed, newRssFeedString, err := RssParseURL(subscription.URL)
		if err != nil {
			p.API.LogError(err.Error())
			return err
		}

		// check each item in new feed to see if they exist in old feed
		// if they do not exist post the new item to the channel and update
		// xml in the subscription object

		postsMade := false
		for _, item := range newRssFeed.ItemList {
			exists := false
			for _, oldItem := range oldRssFeed.ItemList {
				if oldItem.Guid == item.Guid {
					exists = true
				}
			}

			// if the item does not exist post it to the correct channel
			if !exists {
				postsMade = true
				post := item.Title + "\n" + item.Link + "\n" + html2md.Convert(item.Description) + "\n"
				p.createBotPost(subscription.ChannelID, post, model.POST_DEFAULT)
			}
		}

		if postsMade {
			subscription.XML = newRssFeedString
			p.updateSubscription(subscription)
		}

	} else {
		//p.API.LogInfo("Gettings RSS for url = " + subscription.URL)

		newRssFeed, newRssFeedXML, err := RssParseURL(subscription.URL)
		if err != nil {
			p.API.LogError("Go Feed failed to parse new subscription.")
			p.API.LogError(err.Error())

			return err
		}

		//p.API.LogInfo(fmt.Sprintf("New RSS Feed Title %s\n Description %s\n", newRssFeed.Title, newRssFeed.Description))
		//.API.LogInfo(fmt.Sprintf("New RSS Feed Items %v", newRssFeed.ItemList))

		for _, item := range newRssFeed.ItemList {
			post := item.Title + "\n" + item.Link + "\n" + html2md.Convert(item.Description) + "\n"
			p.createBotPost(subscription.ChannelID, post, model.POST_DEFAULT)
		}

		subscription.XML = newRssFeedXML
		p.updateSubscription(subscription)
	}

	return nil
}

func (p *RSSFeedPlugin) createBotPost(channelID string, message string, postType string) error {
	config := p.getConfiguration()
	user, err := p.API.GetUserByUsername(config.Username)
	if err != nil {
		mlog.Error(err.Error())
		return err
	}
	post := &model.Post{
		UserId:    user.Id,
		ChannelId: channelID,
		Message:   message,
		Type:      postType,
		Props: map[string]interface{}{
			"override_username": RSSFEED_USERNAME,
			"override_icon_url": RSSFEED_ICON_URL,
			"from_webhook":      true,
		},
	}

	if _, err := p.API.CreatePost(post); err != nil {
		mlog.Error(err.Error())
		return err
	}

	return nil
}

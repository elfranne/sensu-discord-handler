package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/sensu-community/sensu-plugin-sdk/sensu"
	"github.com/sensu-community/sensu-plugin-sdk/templates"
	"github.com/sensu/sensu-go/types"
)

// Config represents the handler plugin config.
type Config struct {
	sensu.PluginConfig
	discordWebHookURL          string
	discordCustomUsername      string
	discordCustomAvatarURL     string
	discordDescriptionTemplate string
	discordAlertCritical       bool
	discordAlertMention        string
}

const (
	webHookURL          = "webhook-url"
	customUsername      = "custom-username"
	customAvatarURL     = "custom-avatar-url"
	descriptionTemplate = "description-template"
	alertCritical       = "alert-on-critical"
	alertMention        = "alert-mention"

	defaultTemplate     = "{{ .Check.Output }}"
	defaultAlert        = false
	defaultAlertMention = "@everyone"
)

var (
	plugin = Config{
		PluginConfig: sensu.PluginConfig{
			Name:     "sensu-discord-handler",
			Short:    "The Sensu Go Discord handler for notifying a channel.",
			Keyspace: "sensu.io/plugins/sensu-discord-handler/config",
		},
	}

	options = []*sensu.PluginConfigOption{
		{
			Path:      webHookURL,
			Env:       "DISCORD_WEBHOOK_URL",
			Argument:  webHookURL,
			Shorthand: "w",
			Secret:    true,
			Usage:     "The WebHook URL to send messages to",
			Value:     &plugin.discordWebHookURL,
		},
		{
			Path:      customUsername,
			Env:       "DISCORD_CUSTOM_USERNAME",
			Argument:  customUsername,
			Shorthand: "u",
			Default:   "",
			Usage:     "The username that messages will be sent as",
			Value:     &plugin.discordCustomUsername,
		},
		{
			Path:      customAvatarURL,
			Env:       "DISCORD_CUSTOM_AVATAR_URL",
			Argument:  customAvatarURL,
			Shorthand: "i",
			Default:   "",
			Usage:     "A URL to an image to use as the user avatar",
			Value:     &plugin.discordCustomAvatarURL,
		},
		{
			Path:      descriptionTemplate,
			Env:       "DISCORD_DESCRIPTION_TEMPLATE",
			Argument:  descriptionTemplate,
			Shorthand: "t",
			Default:   defaultTemplate,
			Usage:     "The Discord notification output template, in Golang text/template format",
			Value:     &plugin.discordDescriptionTemplate,
		},
		{
			Path:      alertCritical,
			Env:       "DISCORD_ALERT_ON_CRITICAL",
			Argument:  alertCritical,
			Shorthand: "a",
			Default:   defaultAlert,
			Usage:     "The Discord notification will alert the channel with a specified mentions (--alert-mention)",
			Value:     &plugin.discordAlertCritical,
		},
		{
			Path:      alertMention,
			Env:       "DISCORD_ALERT_MENTION",
			Argument:  alertMention,
			Shorthand: "m",
			Default:   defaultAlertMention,
			Usage:     "Specifies the mentions to use if --alert-on-critical is enabled",
			Value:     &plugin.discordAlertMention,
		},
	}
)

func main() {
	handler := sensu.NewGoHandler(&plugin.PluginConfig, options, checkArgs, executeHandler)
	handler.Execute()
}

func checkArgs(_ *types.Event) error {
	if len(plugin.discordWebHookURL) == 0 {
		return fmt.Errorf("--%s or DISCORD_WEBHOOK_URL environment variable is required", webHookURL)
	}

	return nil
}

func formattedEventAction(event *types.Event) string {
	switch event.Check.Status {
	case 0:
		return "RESOLVED"
	default:
		return "ALERT"
	}
}

func chomp(s string) string {
	return strings.Trim(strings.Trim(strings.Trim(s, "\n"), "\r"), "\r\n")
}

func eventKey(event *types.Event) string {
	return fmt.Sprintf("%s/%s", event.Entity.Name, event.Check.Name)
}

func eventSummary(event *types.Event, maxLength int) string {
	output := chomp(event.Check.Output)
	if len(event.Check.Output) > maxLength {
		output = output[0:maxLength] + "..."
	}
	return fmt.Sprintf("%s:%s", eventKey(event), output)
}

func formattedMessage(event *types.Event) string {
	return fmt.Sprintf("%s - %s", formattedEventAction(event), eventSummary(event, 100))
}

func messageColor(event *types.Event) int {
	switch event.Check.Status {
	case 0:
		return 3061373
	case 2:
		return 14687834
	default:
		return 15512110
	}
}

func messageStatus(event *types.Event) string {
	switch event.Check.Status {
	case 0:
		return "Resolved"
	case 2:
		if plugin.discordAlertCritical {
			return fmt.Sprintf("%s Critical", plugin.discordAlertMention)
		} else {
			return "Critical"
		}
	default:
		return "Warning"
	}
}

func messageEmbed(event *types.Event) *discordgo.MessageEmbed {
	description, err := templates.EvalTemplate("description", plugin.discordDescriptionTemplate, event)
	if err != nil {
		fmt.Printf("%s: Error processing template: %s", plugin.PluginConfig.Name, err)
	}

	description = strings.Replace(description, `\n`, "\n", -1)
	embed := &discordgo.MessageEmbed{
		Title:       "Description",
		Description: description,
		Color:       messageColor(event),
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Status",
				Value:  messageStatus(event),
				Inline: false,
			},
			{
				Name:   "Entity",
				Value:  event.Entity.Name,
				Inline: true,
			},
			{
				Name:   "Check",
				Value:  event.Check.Name,
				Inline: true,
			},
		},
	}

	return embed
}

func executeHandler(event *types.Event) error {
	embedJSON, err := json.Marshal(messageEmbed(event))
	if err != nil {
		return fmt.Errorf("%v", err)
	}
	resp, err := http.Post(plugin.discordWebHookURL, "application/json", strings.NewReader(fmt.Sprintf("{\"embeds\": [%s]}", string(embedJSON))))
	if err != nil {
		return fmt.Errorf("%v", err)
	}
	defer resp.Body.Close()

	fmt.Print("Notification sent to Discord WebHook destination\n")

	return nil
}

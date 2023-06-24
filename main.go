package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/sensu/sensu-plugin-sdk/templates"
	"github.com/sensu/sensu-plugin-sdk/sensu"
	corev2 "github.com/sensu/core/v2"
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

	options = []sensu.ConfigOption{
		&sensu.PluginConfigOption[string]{
			Path:      webHookURL,
			Env:       "DISCORD_WEBHOOK_URL",
			Argument:  webHookURL,
			Shorthand: "w",
			Secret:    true,
			Usage:     "The WebHook URL to send messages to",
			Value:     &plugin.discordWebHookURL,
		},
		&sensu.PluginConfigOption[string]{
			Path:      customUsername,
			Env:       "DISCORD_CUSTOM_USERNAME",
			Argument:  customUsername,
			Shorthand: "u",
			Default:   "",
			Usage:     "The username that messages will be sent as",
			Value:     &plugin.discordCustomUsername,
		},
		&sensu.PluginConfigOption[string]{
			Path:      customAvatarURL,
			Env:       "DISCORD_CUSTOM_AVATAR_URL",
			Argument:  customAvatarURL,
			Shorthand: "i",
			Default:   "",
			Usage:     "A URL to an image to use as the user avatar",
			Value:     &plugin.discordCustomAvatarURL,
		},
		&sensu.PluginConfigOption[string]{
			Path:      descriptionTemplate,
			Env:       "DISCORD_DESCRIPTION_TEMPLATE",
			Argument:  descriptionTemplate,
			Shorthand: "t",
			Default:   defaultTemplate,
			Usage:     "The Discord notification output template, in Golang text/template format",
			Value:     &plugin.discordDescriptionTemplate,
		},
		&sensu.PluginConfigOption[bool]{
			Path:      alertCritical,
			Env:       "DISCORD_ALERT_ON_CRITICAL",
			Argument:  alertCritical,
			Shorthand: "a",
			Default:   defaultAlert,
			Usage:     "The Discord notification will alert the channel with a specified mentions (--alert-mention)",
			Value:     &plugin.discordAlertCritical,
		},
		&sensu.PluginConfigOption[string]{
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
	handler := sensu.NewHandler(&plugin.PluginConfig, options, checkArgs, executeHandler)
	handler.Execute()
}

func checkArgs(_ *corev2.Event) error {
	if len(plugin.discordWebHookURL) == 0 {
		return fmt.Errorf("--%s or DISCORD_WEBHOOK_URL environment variable is required", webHookURL)
	}

	return nil
}

func messageColor(event *corev2.Event) int {
	switch event.Check.Status {
	case 0:
		return 3061373
	case 2:
		return 14687834
	default:
		return 15512110
	}
}

func messageStatus(event *corev2.Event) string {
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

func limitTextLength(text string, limit int) string {
	textRune := []rune(text)
	if len(textRune) > limit {
		return string(textRune[0:limit-3]) + "..."
	}
	return text
}

func messageEmbed(event *corev2.Event) *discordgo.MessageEmbed {
	description, err := templates.EvalTemplate("description", plugin.discordDescriptionTemplate, event)
	if err != nil {
		fmt.Printf("%s: Error processing template: %s", plugin.PluginConfig.Name, err)
	}

	description = strings.Replace(description, `\n`, "\n", -1)
	embed := &discordgo.MessageEmbed{
		Title:       "Description",
		Description: limitTextLength(description, 4096),
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

func executeHandler(event *corev2.Event) error {
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

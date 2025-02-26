package discord_bot

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"stable_diffusion_bot/entities"
	"stable_diffusion_bot/imagine_queue"
	"stable_diffusion_bot/stable_diffusion_api"

	"github.com/bwmarrin/discordgo"
)

type botImpl struct {
	developmentMode    bool
	botSession         *discordgo.Session
	guildID            string
	imagineQueue       imagine_queue.Queue
	registeredCommands []*discordgo.ApplicationCommand
	imagineCommand     string
	removeCommands     bool
	stableDiffusionAPI stable_diffusion_api.StableDiffusionAPI
}

type Config struct {
	DevelopmentMode    bool
	BotToken           string
	GuildID            string
	ImagineQueue       imagine_queue.Queue
	ImagineCommand     string
	RemoveCommands     bool
	StableDiffusionAPI stable_diffusion_api.StableDiffusionAPI
}

func (b *botImpl) imagineCommandString() string {
	if b.developmentMode {
		return "dev_" + b.imagineCommand
	}

	return b.imagineCommand
}

func (b *botImpl) imagineExtCommandString() string {
	if b.developmentMode {
		return "dev_" + b.imagineCommand + "_ext"
	}

	return b.imagineCommand + "_ext"
}

func (b *botImpl) imagineSettingsCommandString() string {
	if b.developmentMode {
		return "dev_" + b.imagineCommand + "_settings"
	}

	return b.imagineCommand + "_settings"
}

func (b *botImpl) changeModelCommandString() string {
	if b.developmentMode {
		return "dev_" + b.imagineCommand + "_change_model"
	}

	return b.imagineCommand + "_change_model"
}

func New(cfg Config) (Bot, error) {
	if cfg.BotToken == "" {
		return nil, errors.New("missing bot token")
	}

	if cfg.GuildID == "" {
		return nil, errors.New("missing guild ID")
	}

	if cfg.ImagineQueue == nil {
		return nil, errors.New("missing imagine queue")
	}

	if cfg.ImagineCommand == "" {
		return nil, errors.New("missing imagine command")
	}

	if cfg.StableDiffusionAPI == nil {
		return nil, errors.New("missing stable diffusion API")
	}

	botSession, err := discordgo.New("Bot " + cfg.BotToken)
	if err != nil {
		return nil, err
	}

	botSession.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Printf("Logged in as: %v#%v", s.State.User.Username, s.State.User.Discriminator)
	})

	err = botSession.Open()
	if err != nil {
		return nil, err
	}

	bot := &botImpl{
		developmentMode:    cfg.DevelopmentMode,
		botSession:         botSession,
		imagineQueue:       cfg.ImagineQueue,
		registeredCommands: make([]*discordgo.ApplicationCommand, 0),
		imagineCommand:     cfg.ImagineCommand,
		removeCommands:     cfg.RemoveCommands,
		stableDiffusionAPI: cfg.StableDiffusionAPI,
	}

	err = bot.addImagineCommand()
	if err != nil {
		return nil, err
	}

	err = bot.addImagineExtCommand()
	if err != nil {
		return nil, err
	}

	err = bot.addImagineSettingsCommand()
	if err != nil {
		return nil, err
	}

	err = bot.addChangeModelCommand()
	if err != nil {
		return nil, err
	}

	botSession.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			switch i.ApplicationCommandData().Name {
			case bot.imagineCommandString():
				bot.processImagineCommand(s, i)
			case bot.imagineExtCommandString():
				bot.processImagineExtCommand(s, i)
			case bot.imagineSettingsCommandString():
				bot.processImagineSettingsCommand(s, i)
			case bot.changeModelCommandString():
				bot.processModelSettingsCommand(s, i)
			default:
				log.Printf("Unknown command '%v'", i.ApplicationCommandData().Name)
			}
		case discordgo.InteractionMessageComponent:
			switch customID := i.MessageComponentData().CustomID; {
			case customID == "imagine_reroll":
				bot.processImagineReroll(s, i)
			case strings.HasPrefix(customID, "imagine_upscale_"):
				interactionIndex := strings.TrimPrefix(customID, "imagine_upscale_")

				interactionIndexInt, intErr := strconv.Atoi(interactionIndex)
				if intErr != nil {
					log.Printf("Error parsing interaction index: %v", err)

					return
				}

				bot.processImagineUpscale(s, i, interactionIndexInt)
			case strings.HasPrefix(customID, "imagine_variation_"):
				interactionIndex := strings.TrimPrefix(customID, "imagine_variation_")

				interactionIndexInt, intErr := strconv.Atoi(interactionIndex)
				if intErr != nil {
					log.Printf("Error parsing interaction index: %v", err)

					return
				}

				bot.processImagineVariation(s, i, interactionIndexInt)
			case customID == "imagine_dimension_setting_menu":
				if len(i.MessageComponentData().Values) == 0 {
					log.Printf("No values for imagine dimension setting menu")

					return
				}

				sizes := strings.Split(i.MessageComponentData().Values[0], "_")

				width := sizes[0]
				height := sizes[1]

				widthInt, intErr := strconv.Atoi(width)
				if intErr != nil {
					log.Printf("Error parsing width: %v", err)

					return
				}

				heightInt, intErr := strconv.Atoi(height)
				if intErr != nil {
					log.Printf("Error parsing height: %v", err)

					return
				}

				bot.processImagineDimensionSetting(s, i, widthInt, heightInt)
			case customID == "imagine_batch_count_setting_menu":
				if len(i.MessageComponentData().Values) == 0 {
					log.Printf("No values for imagine batch count setting menu")

					return
				}

				batchCount := i.MessageComponentData().Values[0]

				batchCountInt, intErr := strconv.Atoi(batchCount)
				if intErr != nil {
					log.Printf("Error parsing batch count: %v", err)

					return
				}

				var batchSizeInt int

				// calculate the corresponding batch size
				switch batchCountInt {
				case 1:
					batchSizeInt = 4
				case 2:
					batchSizeInt = 2
				case 4:
					batchSizeInt = 1
				default:
					log.Printf("Unknown batch count: %v", batchCountInt)

					return
				}

				bot.processImagineBatchSetting(s, i, batchCountInt, batchSizeInt)
			case customID == "imagine_batch_size_setting_menu":
				if len(i.MessageComponentData().Values) == 0 {
					log.Printf("No values for imagine batch count setting menu")

					return
				}

				batchSize := i.MessageComponentData().Values[0]

				batchSizeInt, intErr := strconv.Atoi(batchSize)
				if intErr != nil {
					log.Printf("Error parsing batch count: %v", err)

					return
				}

				var batchCountInt int

				// calculate the corresponding batch count
				switch batchSizeInt {
				case 1:
					batchCountInt = 4
				case 2:
					batchCountInt = 2
				case 4:
					batchCountInt = 1
				default:
					log.Printf("Unknown batch size: %v", batchSizeInt)

					return
				}

				bot.processImagineBatchSetting(s, i, batchCountInt, batchSizeInt)
			case customID == "imagine_change_model":
				bot.processChangeModel(s, i)
			default:
				log.Printf("Unknown message component '%v'", i.MessageComponentData().CustomID)
			}
		}
	})

	return bot, nil
}

func (b *botImpl) Start() {
	b.imagineQueue.StartPolling(b.botSession)

	err := b.teardown()
	if err != nil {
		log.Printf("Error tearing down bot: %v", err)
	}
}

func (b *botImpl) teardown() error {
	// Delete all commands added by the bot
	if b.removeCommands {
		log.Printf("Removing all commands added by bot...")

		for _, v := range b.registeredCommands {
			log.Printf("Removing command '%v'...", v.Name)

			err := b.botSession.ApplicationCommandDelete(b.botSession.State.User.ID, b.guildID, v.ID)
			if err != nil {
				log.Panicf("Cannot delete '%v' command: %v", v.Name, err)
			}
		}
	}

	return b.botSession.Close()
}

func (b *botImpl) addImagineCommand() error {
	log.Printf("Adding command '%s'...", b.imagineCommandString())

	cmd, err := b.botSession.ApplicationCommandCreate(b.botSession.State.User.ID, b.guildID, &discordgo.ApplicationCommand{
		Name:        b.imagineCommandString(),
		Description: "Ask the bot to imagine something",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "prompt",
				Description: "The text prompt to imagine",
				Required:    true,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "negative",
				Description: "Negative prompt",
				Required:    false,
			}},
	})
	if err != nil {
		log.Printf("Error creating '%s' command: %v", b.imagineCommandString(), err)

		return err
	}

	b.registeredCommands = append(b.registeredCommands, cmd)

	return nil
}

const (
	extOptionAR             = `aspect_ratio`
	extOptionCFGScale       = `cfg_scale`
	extOptionEmbeddings     = `embeddings`
	extOptionNegativePrompt = `negative_prompt`
	extOptionPrompt         = `prompt`
	extOptionRestoreFaces   = `restore_faces`
	extOptionSampler        = `sampler`
	extOptionSeed           = `seed`
	extOptionSteps          = `steps`
)

func (b *botImpl) addImagineExtCommand() error {
	log.Printf("Adding command '%s'...", b.imagineExtCommandString())

	minNum := 1.0
	commandOptions := []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        extOptionPrompt,
			Description: "The text prompt to imagine (`--ar x:y` to set aspect ratio)",
			Required:    true,
		},
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        extOptionAR,
			Description: "Aspect Ratio",
			Required:    false,
			Choices: []*discordgo.ApplicationCommandOptionChoice{
				{
					Name:  "1:1  (square, 512×512)",
					Value: "",
				},
				{
					Name:  "4:3  (horizontal, 688×512)",
					Value: "--ar 4:3",
				},
				{
					Name:  "16:10 (horizontal wide, 824×512)",
					Value: "--ar 16:10",
				},
				{
					Name:  "16:9 (horizontal wide, 912×512)",
					Value: "--ar 16:9",
				},
				{
					Name:  "3:4 (vertical, 512×688)",
					Value: "--ar 3:4",
				},
				{
					Name:  "10:16 (vertical narrow, 512×824)",
					Value: "--ar 10:16",
				},
				{
					Name:  "9:16 (vertical narrow, 512×912)",
					Value: "--ar 9:16",
				},
			},
		},
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        extOptionNegativePrompt,
			Description: "Negative prompt",
			Required:    false,
		},
		{
			Type:        discordgo.ApplicationCommandOptionBoolean,
			Name:        extOptionRestoreFaces,
			Description: "Restore faces" + fmt.Sprintf(" (%v)", imagine_queue.DefaultRestoreFaces),
			Required:    false,
		},
		{
			Type:        discordgo.ApplicationCommandOptionNumber,
			Name:        extOptionCFGScale,
			Description: fmt.Sprintf("CFG Scale (%d)", imagine_queue.DefaultCFGScale),
			Required:    false,
			MinValue:    &minNum,
			MaxValue:    30,
		},
		{
			Type:        discordgo.ApplicationCommandOptionInteger,
			Name:        extOptionSeed,
			Description: fmt.Sprintf("Seed (%d)", imagine_queue.DefaultSeed),
			Required:    false,
		},
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        extOptionSampler,
			Description: fmt.Sprintf("Sampler (%s)", imagine_queue.DefaultSampler),
			Required:    false,
			Choices: []*discordgo.ApplicationCommandOptionChoice{
				// TODO: move to config
				{
					Name:  "Euler a",
					Value: "Euler a",
				},
				{
					Name:  "DPM SDE",
					Value: "DPM SDE",
				},
			},
		},
		{
			Type:        discordgo.ApplicationCommandOptionInteger,
			Name:        extOptionSteps,
			Description: fmt.Sprintf("Sampling Steps (%d)", imagine_queue.DefaultSteps),
			MinValue:    &minNum,
			MaxValue:    50,
		},
	}

	// TODO: reload embeddings on model change
	embs, err := b.stableDiffusionAPI.GetEmbeddings()
	if err != nil {
		log.Printf("Error getting embeddings: %v", err)
	}
	if len(embs.Loaded) > 0 {
		var options []*discordgo.ApplicationCommandOptionChoice
		for embed := range embs.Loaded {
			options = append(options, &discordgo.ApplicationCommandOptionChoice{
				Name:  embed,
				Value: embed,
			})

			// Max 25 choices
			// https://discord.com/developers/docs/interactions/application-commands#application-command-object-application-command-option-structure
			if len(options) == 25 {
				log.Printf("Loaded 25/%d textual inversions...", len(embs.Loaded))
				break
			}
		}

		commandOptions = append(commandOptions, &discordgo.ApplicationCommandOption{
			Type:         discordgo.ApplicationCommandOptionString,
			Name:         extOptionEmbeddings,
			Description:  "Textual Inversion",
			Required:     false,
			Autocomplete: false,
			Choices:      options,
		})
	}

	cmd, err := b.botSession.ApplicationCommandCreate(b.botSession.State.User.ID, b.guildID, &discordgo.ApplicationCommand{
		Name:        b.imagineExtCommandString(),
		Description: "Ask the bot to imagine something",
		Options:     commandOptions,
	})
	if err != nil {
		log.Printf("Error creating '%s' command: %v", b.imagineExtCommandString(), err)
		return err
	}

	b.registeredCommands = append(b.registeredCommands, cmd)

	return nil
}

func (b *botImpl) addImagineSettingsCommand() error {
	log.Printf("Adding command '%s'...", b.imagineSettingsCommandString())

	cmd, err := b.botSession.ApplicationCommandCreate(b.botSession.State.User.ID, b.guildID, &discordgo.ApplicationCommand{
		Name:        b.imagineSettingsCommandString(),
		Description: "Change the default settings for the imagine command",
	})
	if err != nil {
		log.Printf("Error creating '%s' command: %v", b.imagineSettingsCommandString(), err)
		return err
	}

	b.registeredCommands = append(b.registeredCommands, cmd)

	return nil
}

func (b *botImpl) addChangeModelCommand() error {
	log.Printf("Adding command '%s'...", b.changeModelCommandString())

	cmd, err := b.botSession.ApplicationCommandCreate(b.botSession.State.User.ID, b.guildID, &discordgo.ApplicationCommand{
		Name:        b.changeModelCommandString(),
		Description: "Change the model used for the imagine command",
	})
	if err != nil {
		log.Printf("Error creating '%s' command: %v", b.changeModelCommandString(), err)
		return err
	}

	b.registeredCommands = append(b.registeredCommands, cmd)

	return nil
}

func (b *botImpl) processChangeModel(s *discordgo.Session, i *discordgo.InteractionCreate) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		log.Printf("Error deferring interaction: %v", err)
		return
	}

	selectedModel := i.MessageComponentData().Values[0] // grab the value from selected model

	// set selected model via stable diffusion API
	if err := b.stableDiffusionAPI.SetSelectedModel(selectedModel); err != nil {
		log.Printf("Failed to post selected model: %v", err)
		s.FollowupMessageCreate(i.Interaction, false, &discordgo.WebhookParams{
			Content: "Error updating the model. Please try again.",
		})
		return
	}

	s.FollowupMessageCreate(i.Interaction, false, &discordgo.WebhookParams{
		Content: "Model updated successfully.",
	})

}

func (b *botImpl) processImagineReroll(s *discordgo.Session, i *discordgo.InteractionCreate) {
	position, queueError := b.imagineQueue.AddImagine(&imagine_queue.QueueItem{
		Type:               imagine_queue.ItemTypeReroll,
		DiscordInteraction: i.Interaction,
	})
	if queueError != nil {
		log.Printf("Error adding imagine to queue: %v\n", queueError)
	}

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("I'm reimagining that for you... You are currently #%d in line.", position),
		},
	})
	if err != nil {
		log.Printf("Error responding to interaction: %v", err)
	}
}

func (b *botImpl) processImagineUpscale(s *discordgo.Session, i *discordgo.InteractionCreate, upscaleIndex int) {
	position, queueError := b.imagineQueue.AddImagine(&imagine_queue.QueueItem{
		Type:               imagine_queue.ItemTypeUpscale,
		InteractionIndex:   upscaleIndex,
		DiscordInteraction: i.Interaction,
	})
	if queueError != nil {
		log.Printf("Error adding imagine to queue: %v\n", queueError)
	}

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("I'm upscaling that for you... You are currently #%d in line.", position),
		},
	})
	if err != nil {
		log.Printf("Error responding to interaction: %v", err)
	}
}

func (b *botImpl) processImagineVariation(s *discordgo.Session, i *discordgo.InteractionCreate, variationIndex int) {
	position, queueError := b.imagineQueue.AddImagine(&imagine_queue.QueueItem{
		Type:               imagine_queue.ItemTypeVariation,
		InteractionIndex:   variationIndex,
		DiscordInteraction: i.Interaction,
	})
	if queueError != nil {
		log.Printf("Error adding imagine to queue: %v\n", queueError)
	}

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("I'm imagining more variations for you... You are currently #%d in line.", position),
		},
	})
	if err != nil {
		log.Printf("Error responding to interaction: %v", err)
	}
}

func (b *botImpl) processImagineCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options

	optionMap := make(map[string]*discordgo.ApplicationCommandInteractionDataOption, len(options))
	for _, opt := range options {
		optionMap[opt.Name] = opt
	}

	var position int
	var queueError error
	var prompt string
	var negative string

	if option, ok := optionMap["prompt"]; ok {
		prompt = option.StringValue()
	}

	if option, ok := optionMap["negative"]; ok {
		negative = option.StringValue()
	}

	if negative != "" && prompt != "" {

		position, queueError = b.imagineQueue.AddImagine(&imagine_queue.QueueItem{
			Prompt:             prompt,
			Options:            imagine_queue.NewQueueItemOptions(),
			Type:               imagine_queue.ItemTypeImagine,
			DiscordInteraction: i.Interaction,
		})
	} else if prompt != "" {
		position, queueError = b.imagineQueue.AddImagine(&imagine_queue.QueueItem{
			Prompt:             prompt,
			Options:            imagine_queue.NewQueueItemOptions(),
			Type:               imagine_queue.ItemTypeImagine,
			DiscordInteraction: i.Interaction,
		})
	}

	if queueError != nil {
		log.Printf("Error adding imagine to queue: %v\n", queueError)
	}

	userID := ""

	if i.Member != nil {
		userID = i.Member.User.ID
	} else if i.User != nil {
		userID = i.User.ID
	}

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf(
				"I'm dreaming something up for you. You are currently #%d in line.\n<@%s> asked me to imagine \"%s\" without \"%s\".",
				position,
				userID,
				prompt,
				negative),
		},
	})
	if err != nil {
		log.Printf("Error responding to interaction: %v", err)
	}
}

func settingsMessageComponents(settings *entities.DefaultSettings) []discordgo.MessageComponent {
	minValues := 1

	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{
					CustomID:  "imagine_dimension_setting_menu",
					MinValues: &minValues,
					MaxValues: 1,
					Options: []discordgo.SelectMenuOption{
						{
							Label:   "Size: 512x512",
							Value:   "512_512",
							Default: settings.Width == 512 && settings.Height == 512,
						},
						{
							Label:   "Size: 768x768",
							Value:   "768_768",
							Default: settings.Width == 768 && settings.Height == 768,
						},
						{
							Label:   "Size: 1024x1024",
							Value:   "1024_1024",
							Default: settings.Width == 1024 && settings.Height == 1024,
						},
					},
				},
			},
		},
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{
					CustomID:  "imagine_batch_count_setting_menu",
					MinValues: &minValues,
					MaxValues: 1,
					Options: []discordgo.SelectMenuOption{
						{
							Label:   "Batch count: 1",
							Value:   "1",
							Default: settings.BatchCount == 1,
						},
						{
							Label:   "Batch count: 2",
							Value:   "2",
							Default: settings.BatchCount == 2,
						},
						{
							Label:   "Batch count: 4",
							Value:   "4",
							Default: settings.BatchCount == 4,
						},
					},
				},
			},
		},
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{
					CustomID:  "imagine_batch_size_setting_menu",
					MinValues: &minValues,
					MaxValues: 1,
					Options: []discordgo.SelectMenuOption{
						{
							Label:   "Batch size: 1",
							Value:   "1",
							Default: settings.BatchSize == 1,
						},
						{
							Label:   "Batch size: 2",
							Value:   "2",
							Default: settings.BatchSize == 2,
						},
						{
							Label:   "Batch size: 4",
							Value:   "4",
							Default: settings.BatchSize == 4,
						},
					},
				},
			},
		},
	}
}

func (b *botImpl) processImagineExtCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options

	queueOptions := imagine_queue.NewQueueItemOptions()
	aspectRatio := ""
	for _, opt := range options {
		switch opt.Name {
		case extOptionAR:
			aspectRatio = opt.StringValue()

			if aspectRatio != "" {
				queueOptions.Prompt += ` ` + aspectRatio
			}
		case extOptionPrompt:
			queueOptions.Prompt = opt.StringValue()
		case extOptionNegativePrompt:
			queueOptions.NegativePrompt = opt.StringValue()
		case extOptionRestoreFaces:
			queueOptions.RestoreFaces = opt.BoolValue()
		case extOptionCFGScale:
			queueOptions.CfgScale = opt.FloatValue()
		case extOptionSeed:
			queueOptions.Seed = int(opt.IntValue())
		case extOptionSampler:
			queueOptions.SamplerName = opt.StringValue()
		case extOptionEmbeddings:
			queueOptions.Prompt += `, ` + opt.StringValue()
		case extOptionSteps:
			queueOptions.Steps = int(opt.IntValue())
		}
	}

	var position int
	var queueError error

	userID := ""

	if i.Member != nil {
		userID = i.Member.User.ID
	} else if i.User != nil {
		userID = i.User.ID
	}

	position, queueError = b.imagineQueue.AddImagine(&imagine_queue.QueueItem{
		Prompt:             queueOptions.Prompt,
		Options:            queueOptions,
		Type:               imagine_queue.ItemTypeImagine,
		DiscordInteraction: i.Interaction,
	})

	if queueError != nil {
		log.Printf("Error adding imagine to queue: %v\n", queueError)
	}

	message := fmt.Sprintf(
		"I'm dreaming something up for you. You are currently #%d in line.\n<@%s> asked me to imagine `%s`.",
		position,
		userID,
		queueOptions.Prompt,
	)

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: message,
		},
	})
	if err != nil {
		log.Printf("Error send interaction resp: %v\n", err)
	}
}

func (b *botImpl) processImagineSettingsCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	botSettings, err := b.imagineQueue.GetBotDefaultSettings()
	if err != nil {
		log.Printf("error getting default settings for settings command: %v", err)

		return
	}

	messageComponents := settingsMessageComponents(botSettings)

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Title:      "Settings",
			Content:    "Choose defaults settings for the imagine command:",
			Components: messageComponents,
		},
	})
	if err != nil {
		log.Printf("Error responding to interaction: %v", err)
	}
}

func (b *botImpl) processModelSettingsCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	modelTitles, err := b.stableDiffusionAPI.GetModels()
	if err != nil {
		log.Printf("Error fetching model titles: %v", err)
		return
	}

	messageComponents := changeModelMessageComponents(modelTitles)

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content:    "Choose a model for the imagine command:",
			Components: messageComponents,
		},
	})
	if err != nil {
		log.Printf("Error responding to interaction: %v", err)
	}
}

func changeModelMessageComponents(models []string) []discordgo.MessageComponent {
	minValues := 1

	options := make([]discordgo.SelectMenuOption, len(models))
	for i, title := range models {
		options[i] = discordgo.SelectMenuOption{
			Label: title,
			Value: title,
		}

		// Discord limits the number of choices to 25
		if len(options) == 25 {
			log.Printf("Loaded 25/%d models...", len(models))
			break
		}
	}

	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{
					CustomID:  "imagine_change_model",
					MinValues: &minValues,
					MaxValues: 1,
					Options:   options,
				},
			},
		},
	}
}

func (b *botImpl) processImagineDimensionSetting(s *discordgo.Session, i *discordgo.InteractionCreate, height, width int) {
	botSettings, err := b.imagineQueue.UpdateDefaultDimensions(width, height)
	if err != nil {
		log.Printf("error updating default dimensions: %v", err)

		err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content: "Error updating default dimensions...",
			},
		})
		if err != nil {
			log.Printf("Error responding to interaction: %v", err)
		}

		return
	}

	messageComponents := settingsMessageComponents(botSettings)

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content:    "Choose defaults settings for the imagine command:",
			Components: messageComponents,
		},
	})
	if err != nil {
		log.Printf("Error responding to interaction: %v", err)
	}
}

func (b *botImpl) processImagineBatchSetting(s *discordgo.Session, i *discordgo.InteractionCreate, batchCount, batchSize int) {
	botSettings, err := b.imagineQueue.UpdateDefaultBatch(batchCount, batchSize)
	if err != nil {
		log.Printf("error updating batch settings: %v", err)

		err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content: "Error updating batch settings...",
			},
		})
		if err != nil {
			log.Printf("Error responding to interaction: %v", err)
		}

		return
	}

	messageComponents := settingsMessageComponents(botSettings)

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content:    "Choose defaults settings for the imagine command:",
			Components: messageComponents,
		},
	})
	if err != nil {
		log.Printf("Error responding to interaction: %v", err)
	}
}

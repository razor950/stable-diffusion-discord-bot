package main

import (
	"context"
	"flag"
	"log"
	"stable_diffusion_bot/databases/sqlite"
	"stable_diffusion_bot/discord_bot"
	"stable_diffusion_bot/imagine_queue"
	"stable_diffusion_bot/repositories/image_generations"
	"stable_diffusion_bot/stable_diffusion_api"
)

// Bot parameters
var (
	guildID  = flag.String("guild", "", "Guild ID. If not passed - bot registers commands globally")
	botToken = flag.String("token", "", "Bot access token")
	apiHost  = flag.String("host", "", "Host for the Automatic1111 API")
)

func main() {
	flag.Parse()

	if guildID == nil {
		log.Fatalf("Guild ID flag is required")
	}

	if botToken == nil {
		log.Fatalf("Bot token flag is required")
	}

	if apiHost == nil {
		log.Fatalf("API host flag is required")
	}

	stableDiffusionAPI, err := stable_diffusion_api.New(stable_diffusion_api.Config{
		Host: *apiHost,
	})
	if err != nil {
		log.Fatalf("Failed to create Stable Diffusion API: %v", err)
	}

	ctx := context.Background()

	sqliteDB, err := sqlite.New(ctx)
	if err != nil {
		log.Fatalf("Failed to create sqlite database: %v", err)
	}

	generationRepo, err := image_generations.NewRepository(&image_generations.Config{DB: sqliteDB})
	if err != nil {
		log.Fatalf("Failed to create image generation repository: %v", err)
	}

	imagineQueue, err := imagine_queue.New(imagine_queue.Config{
		StableDiffusionAPI:  stableDiffusionAPI,
		ImageGenerationRepo: generationRepo,
	})
	if err != nil {
		log.Fatalf("Failed to create imagine queue: %v", err)
	}

	bot, err := discord_bot.New(discord_bot.Config{
		BotToken:     *botToken,
		GuildID:      *guildID,
		ImagineQueue: imagineQueue,
	})
	if err != nil {
		log.Fatalf("Error creating Discord bot: %v", err)
	}

	log.Println("Press Ctrl+C to exit")

	bot.Start()

	log.Println("Gracefully shutting down.")
}

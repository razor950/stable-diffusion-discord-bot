package stable_diffusion_api

type StableDiffusionAPI interface {
	TextToImage(req *TextToImageRequest) (*TextToImageResponse, error)
	UpscaleImage(upscaleReq *UpscaleRequest) (*UpscaleResponse, error)
	GetCurrentProgress() (*ProgressResponse, error)
	GetEmbeddings() (*EmbeddingsResponseMinimal, error)
	GetModels() ([]string, error)
	SetSelectedModel(string) error
}

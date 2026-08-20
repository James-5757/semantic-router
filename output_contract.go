package semanticrouter

import "strings"

// OutputContractKind describes the format the caller actually expects back.
// Routing must use this contract rather than treating every mention of a media
// domain as a request to invoke that domain's native generation model.
type OutputContractKind string

const (
	OutputContractUnknown OutputContractKind = "unknown"
	OutputContractText    OutputContractKind = "text"
	OutputContractAsset   OutputContractKind = "asset"
)

// OutputContract is deliberately small so it can be logged alongside the
// existing route trace without exposing prompt content.
type OutputContract struct {
	Kind   OutputContractKind
	Domain string
	Reason string
}

// detectOutputContract separates a requested binary/native asset from text
// that describes, prompts, scripts, or plans that asset. It is conservative:
// an asset is returned only for an explicit request to render or return one.
func detectOutputContract(prompt string) OutputContract {
	lower := strings.ToLower(prompt)
	domain := outputContractDomain(lower)
	if domain == "" {
		return OutputContract{Kind: OutputContractUnknown}
	}

	if hasOutputContractTextArtifact(lower) && !hasExplicitAssetOutput(lower) {
		return OutputContract{Kind: OutputContractText, Domain: domain, Reason: "text_artifact_requested"}
	}
	if hasExplicitAssetOutput(lower) || hasNativeAssetGeneration(lower) {
		return OutputContract{Kind: OutputContractAsset, Domain: domain, Reason: "asset_output_requested"}
	}
	return OutputContract{Kind: OutputContractUnknown, Domain: domain}
}

func outputContractDomain(prompt string) string {
	hasOutputAction := outputContractContainsAny(prompt, "生成", "出图", "返回", "输出", "导出", "render", "generate", "return", "create")
	switch {
	case outputContractContainsAny(prompt, "文生图", "图片生成", "图像生成", "ai绘画", "ai 绘画", "ai 画图", "midjourney", "stable diffusion", "dall-e", "dalle", "flux", "text to image", "text-to-image", "image generation") ||
		(hasOutputAction && outputContractContainsAny(prompt, "图片", "图像", "海报", "插画", "image", "poster", "illustration")):
		return "image_generation"
	case outputContractContainsAny(prompt, "视频生成", "文生视频", "text to video", "text-to-video", "video generation") ||
		(hasOutputAction && outputContractContainsAny(prompt, "视频", "video")):
		return "video_generation"
	case outputContractContainsAny(prompt, "语音生成", "文本转语音", "text to speech", "text-to-speech", "audio generation") ||
		(hasOutputAction && outputContractContainsAny(prompt, "音频", "语音", "audio", "speech")):
		return "audio_generation"
	case outputContractContainsAny(prompt, "word 文件", "docx 文件", "pdf 文件", "document file"):
		return "document_file"
	default:
		return ""
	}
}

func hasOutputContractTextArtifact(prompt string) bool {
	return outputContractContainsAny(prompt,
		"关键词", "关键字", "提示词", "提示语", "描述词", "标签", "文案", "脚本", "大纲", "模板", "说明", "alt text",
		"tag", "tags", "prompt", "prompts", "image prompt", "caption", "copy", "script", "outline", "template", "description",
	)
}

func hasExplicitAssetOutput(prompt string) bool {
	return outputContractContainsAny(prompt,
		"直接出图", "直接生成图片", "生成图片文件", "返回图片", "输出图片", "并生成图片", "同时生成图片",
		"直接生成视频", "返回视频", "输出视频", "生成音频文件", "返回音频", "导出 pdf", "生成 word 文件", "生成 docx 文件", "生成一个 docx 文件",
		"generate the image", "return an image", "create the image file", "generate the video", "return a video", "generate an audio file", "export pdf", "create a docx file", "generate a docx file",
	)
}

// hasNativeAssetGeneration covers natural variants such as "生成一张图片".
// It is checked after textual artifacts so "生成图片的提示词" remains a text
// request unless the caller also asks to directly return the image.
func hasNativeAssetGeneration(prompt string) bool {
	hasGenerationVerb := outputContractContainsAny(prompt,
		"生成一张", "生成一幅", "生成一段", "生成一个", "生成图片", "生成视频", "生成音频",
		"generate a", "generate an", "create a", "create an",
	)
	hasMediaObject := outputContractContainsAny(prompt,
		"图片", "图像", "海报", "插画", "image", "poster", "illustration",
		"视频", "video", "音频", "语音", "audio", "speech", "pdf", "word", "docx",
	)
	return hasGenerationVerb && hasMediaObject
}

func outputContractContainsAny(prompt string, values ...string) bool {
	for _, value := range values {
		if strings.Contains(prompt, value) {
			return true
		}
	}
	return false
}

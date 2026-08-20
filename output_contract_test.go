package semanticrouter

import "testing"

func TestDetectOutputContract(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
		kind   OutputContractKind
		domain string
	}{
		{"Chinese image keywords", "给我文生图的关键词，主题是未来城市", OutputContractText, "image_generation"},
		{"English image prompt", "Write tags and a prompt for a text-to-image generator", OutputContractText, "image_generation"},
		{"poster copy is text", "为 AI 绘画海报写一段文案和描述词", OutputContractText, "image_generation"},
		{"explicit image output", "直接生成一张未来城市图片", OutputContractAsset, "image_generation"},
		{"mixed image request returns asset", "给我关键词，并直接生成图片", OutputContractAsset, "image_generation"},
		{"video prompt is text", "写一段文生视频提示词和分镜脚本", OutputContractText, "video_generation"},
		{"explicit video output", "直接生成一段产品演示视频", OutputContractAsset, "video_generation"},
		{"audio narration is text", "为文本转语音准备一份旁白脚本", OutputContractText, "audio_generation"},
		{"explicit audio output", "生成音频文件并返回下载链接", OutputContractAsset, "audio_generation"},
		{"document outline is text", "给我一个 Word 文件的报告大纲和模板", OutputContractText, "document_file"},
		{"explicit document output", "生成一个 docx 文件", OutputContractAsset, "document_file"},
		{"ordinary chat stays unknown", "解释一下什么是扩散模型", OutputContractUnknown, ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := detectOutputContract(test.prompt)
			if got.Kind != test.kind || got.Domain != test.domain {
				t.Fatalf("contract=%+v, want kind=%s domain=%s", got, test.kind, test.domain)
			}
		})
	}
}

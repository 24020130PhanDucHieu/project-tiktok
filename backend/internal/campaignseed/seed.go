// Package campaignseed holds the campaigns that exist from the first start, and the pictures they use.
package campaignseed

import "embed"

//go:embed *.png
var files embed.FS

// Step is one message of a seeded campaign. A picture step names its file in this package.
type Step struct {
	Kind, Text, Caption, File string
}

// Campaign is a seeded campaign.
type Campaign struct {
	Name  string
	Steps []Step
}

func text(s string) Step     { return Step{Kind: "text", Text: s} }
func image(file string) Step { return Step{Kind: "image", File: file} }

// Campaigns are the three campaigns offered at the start. The first one is the invitation the team sends by hand today.
func Campaigns() []Campaign {
	return []Campaign{
		{Name: "Liên hệ KOC mới", Steps: []Step{
			text("Chào bạn"),
			text("Chúng mình xin giới thiệu một chút. Chúng mình là thương hiệu đồ thể thao Cooltech. Bên mình là một thương hiệu mới với ưu thế nổi bật ở phần chất liệu: Bên mình tự nghiên cứu và phát triển (R&D) công thức vải, sau đó đặt sản xuất độc quyền, nên kiểm soát được chất lượng từ chất liệu đến thành phẩm"),
			text("Chúng mình có theo dõi kênh {kenh} và nhận thấy định hướng nội dung, hình ảnh của bạn khá phù hợp với hướng phát triển mà Cooltech đang xây dựng. Vì vậy, chúng mình mong muốn đề xuất hợp tác cùng bạn với sản phẩm khăn lạnh thể thao Cooltech của bên mình:https://vt.tiktok.com/ZS9hwE5n8G8AU-9iH4C/\nSau khi nhận sản phẩm, bạn có thể lên video review/giới thiệu sản phẩm, hoặc đưa vào tuyến nội dung sử dụng sản phẩm thật theo phong cách của bạn thường làm."),
			image("anh-san-pham.png"),
			image("loi-moi-hop-tac.png"),
			text("Mình gửi lại thông tin SP nha"),
			text("Tuy rằng sản phẩm bên mình là SP mới ra nhưng từ 1/8 - 31/10 bên mình đã bán được gần 4000 khăn và có đánh giá SP là 4.9/5 sao trên cả 2 sàn Shopee và TikTok. Các bạn KOC và khách hàng bên mình đều có nhận xét và trải nghiệm rất tốt về SP của bên mình nha"),
			text("Chúng mình ngỏ ý hợp tác với bạn theo hình thức freecast theo mức hoa hồng là 10% - 3%. Đây là mức hoa hồng hợp tác ban đầu và sẽ tăng dần theo hiệu quả hoạt động giữa bên mình và bạn."),
			text("Có gì cậu dành chút thời gian của cậu để check thông tin hợp tác và phản hồi cho bên mình biết với nha"),
		}},
		{Name: "Nhắc KOC phản hồi thông tin hợp tác", Steps: []Step{
			text("Chào {ten}, bên mình là Cooltech, hôm trước bên mình có gửi bạn thông tin hợp tác về sản phẩm khăn lạnh thể thao. Bạn xem qua rồi phản hồi giúp bên mình nha."),
		}},
		{Name: "Gửi thêm thông tin sản phẩm", Steps: []Step{
			text("Mình gửi thêm thông tin sản phẩm để bạn tham khảo nha"),
			image("anh-san-pham.png"),
			image("loi-moi-hop-tac.png"),
		}},
	}
}

// Image returns the bytes of a seeded picture.
func Image(file string) ([]byte, error) { return files.ReadFile(file) }

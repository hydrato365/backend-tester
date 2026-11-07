package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// 🌟 မင်းရဲ့ Cloudflare R2 အချက်အလက်တွေ ဒီမှာ အစားထိုးပါ
const (
	R2AccountID  = "75e857ca8f9c2863bb7a10a2df71ef40"
	R2AccessKey  = "0261c9b3b5742c2dd800308af51734bd"
	R2SecretKey  = "acd5634cc6c1fc93dd73798376b49d0c285f5544a5caefcf4f22d7bbed263e0b"
	R2BucketName = "noxtan-storage-dev" // R2 မှာ ဆောက်ထားတဲ့ Bucket နာမည်
)

var s3Client *s3.Client
var db *gorm.DB // Database ကို သိမ်းထားမည့် Variable

// 🌟 Database ဇယားများ (Tables) ၏ ပုံစံများ
type User struct {
	ID           string    `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Username     string    `json:"username"`
	PhoneNo      string    `json:"phone_no"`
	Role         string    `json:"role"`
	StarsBalance int       `json:"stars_balance"`
	AvatarUrl    string    `json:"avatar_url"`
	CreatedAt    time.Time `json:"created_at"`
}

type Video struct {
	ID           string    `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	CreatorID    string    `json:"creator_id"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	VideoUrl     string    `json:"video_url"`
	ThumbnailUrl string    `json:"thumbnail_url"`
	Views        int       `json:"views"`
	IsPremium    bool      `json:"is_premium"`
	CreatedAt    time.Time `json:"created_at"`
}

func init() {
	// R2 နဲ့ ချိတ်ဆက်ခြင်း
	r2Resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		return aws.Endpoint{
			URL: fmt.Sprintf("https://%s.r2.cloudflarestorage.com", R2AccountID),
		}, nil
	})

	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithEndpointResolverWithOptions(r2Resolver),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(R2AccessKey, R2SecretKey, "")),
		config.WithRegion("auto"),
	)
	if err != nil {
		log.Fatal("R2 ချိတ်ဆက်မှု မှားယွင်းနေပါသည်:", err)
	}

	// 🌟 Cloudflare R2 အတွက် UsePathStyle = true မပါမဖြစ် ထည့်ပေးရပါမည်
	s3Client = s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})
	log.Println("✅ Cloudflare R2 ချိတ်ဆက်မှု အောင်မြင်ပါသည်")
}

func main() {
	// 🌟 ၁။ Supabase PostgreSQL (Direct Connection: Port 5432) သို့ ချိတ်ဆက်ခြင်း
	dsn := "postgresql://postgres.ewevqedvcldvreyqnbbr:aeiouminnyima5U@aws-0-ap-southeast-2.pooler.supabase.com:5432/postgres"
	var err error

	db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})

	if err != nil {
		log.Fatal("Database ချိတ်ဆက်မှု ကျရှုံးပါသည်:", err)
	}
	log.Println("✅ Supabase Database ချိတ်ဆက်မှု အောင်မြင်ပါသည်")

	// ဇယားများ မရှိသေးပါက အလိုအလျောက် တည်ဆောက်ပေးမည်
	db.AutoMigrate(&User{}, &Video{})

	// 🌟 Foreign Key Error မတက်စေရန် System User (Admin) တစ်ယောက်ကို ကြိုတင် တည်ဆောက်ထားမည်
	systemUser := User{
		ID:           "00000000-0000-0000-0000-000000000000",
		Username:     "Khit Admin",
		PhoneNo:      "09000000000",
		Role:         "admin",
		StarsBalance: 0,
		AvatarUrl:    "https://ui-avatars.com/api/?name=Khit+Admin&background=DA251D&color=fff",
		CreatedAt:    time.Now(),
	}
	// Error ကို ကျော်ရန် FirstOrCreate ဖြင့် စစ်ဆေး၍ ထည့်မည် (ရှိပြီးသားဆိုရင် မထည့်ပါ)
	db.Where(User{ID: systemUser.ID}).FirstOrCreate(&systemUser)

	r := gin.Default()

	// 🌟 Website (Browser) မှ လှမ်းခေါ်ခွင့်ရစေရန် CORS ဖွင့်ပေးခြင်း
	r.Use(cors.Default())

	// 🌟 ၂။ App ရဲ့ Home Screen မှာ ပြဖို့ ဗီဒီယိုစာရင်းကို ဆွဲထုတ်မည့် API
	r.GET("/videos", func(c *gin.Context) {
		var videos []Video
		// Database ထဲက ဗီဒီယိုတွေကို အသစ်တင်တဲ့အစဉ်လိုက် ဆွဲထုတ်မယ်
		db.Order("created_at desc").Find(&videos)
		c.JSON(http.StatusOK, videos)
	})

	// 🌟 ID အလိုက် ဗီဒီယို တစ်ခုချင်းစီကို ဆွဲထုတ်မည့် API (Watch Page အတွက်)
	r.GET("/videos/:id", func(c *gin.Context) {
		id := c.Param("id")
		var video Video
		if err := db.Where("id = ?", id).First(&video).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "ဗီဒီယို ရှာမတွေ့ပါ"})
			return
		}
		c.JSON(http.StatusOK, video)
	})

	// Upload API
	r.POST("/upload", func(c *gin.Context) {
		// ၁။ ဖုန်းက ပို့လိုက်တဲ့ ဗီဒီယို (MP4) နဲ့ စာသားများကို လက်ခံမယ်
		title := c.PostForm("title")
		if title == "" {
			title = "အမည်မသိ ဗီဒီယို"
		}
		description := c.PostForm("description")

		file, err := c.FormFile("video")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "ဗီဒီယို ဖိုင် မပါဝင်ပါ"})
			return
		}

		// ၂။ ယာယီ သိမ်းမယ့် Folder တွေ ဆောက်မယ်
		videoID := fmt.Sprintf("vid_%d", time.Now().Unix()) // 🌟 အချိန်ကို စက္ကန့်နဲ့ ယူပြီး သေသပ်တဲ့ ID ပြောင်းလိုက်ပါပြီ
		tempDir := "./temp_" + videoID
		os.MkdirAll(tempDir, os.ModePerm)
		defer os.RemoveAll(tempDir) // အလုပ်လုပ်ပြီးရင် ဒီ Folder ကို အလိုအလျောက် ပြန်ဖျက်မယ်

		inputPath := filepath.Join(tempDir, "input.mp4")
		c.SaveUploadedFile(file, inputPath)

		// ၃။ (က) AES-128 Encryption အတွက် ၁၆ လုံးပါ သော့ (Key) ဖန်တီးခြင်း
		key := make([]byte, 16)
		rand.Read(key)
		keyPath := filepath.Join(tempDir, "enc.key")
		os.WriteFile(keyPath, key, 0644)

		// ၃။ (ခ) ExoPlayer လှမ်းတောင်းမည့် Key URL ဖန်တီးခြင်း
		// 🌟 အောက်ကလင့်ခ်နေရာမှာ အစ်ကို့ R2.dev လင့်ခ်အစစ်ကို အစားထိုးပေးပါ
		keyUrl := fmt.Sprintf("https://pub-d56c2df021154436960a29402e69ff18.r2.dev/%s/enc.key", videoID)
		keyInfoPath := filepath.Join(tempDir, "key_info.txt")
		keyInfoContent := fmt.Sprintf("%s\n%s\n", keyUrl, keyPath)
		os.WriteFile(keyInfoPath, []byte(keyInfoContent), 0644)

		// ၃။ (ဂ) FFmpeg ဖြင့် Watermark ထည့်ခြင်း၊ ဖိုင်ဆိုက်ချုံ့ခြင်း နှင့် သော့ခတ်ခြင်း
		log.Println("⏳ FFmpeg ဖြင့် HLS ပြောင်းနေပါသည်...")
		masterM3u8Path := filepath.Join(tempDir, "master.m3u8")

		cmd := exec.Command("ffmpeg",
			"-i", inputPath,
			// 🌟 Khit.to Watermark ကို ညာဘက်အပေါ်ထောင့် (Opacity 50%) ဖြင့် ထည့်ခြင်း
			"-vf", "scale=w=1280:h=720:force_original_aspect_ratio=decrease,drawtext=text='Khit.to':fontcolor=white@0.5:fontsize=24:x=W-tw-20:y=20",
			// 🌟 YouTube လို ဖိုင်ဆိုက် သေးသွားအောင် ဖိသိပ်အား (CRF 28) သုံးခြင်း
			"-c:v", "libx264", "-crf", "28", "-preset", "veryfast",
			"-c:a", "aac", "-ar", "48000", "-b:a", "96k",
			// 🌟 HLS 4s ဖြတ်ခြင်း
			"-g", "48", "-keyint_min", "48", "-sc_threshold", "0",
			"-hls_time", "4",
			"-hls_playlist_type", "vod",
			// 🌟 AES-128 ဖြင့် ဗီဒီယိုကို သော့ခတ်ခြင်း
			"-hls_key_info_file", keyInfoPath,
			"-hls_segment_filename", filepath.Join(tempDir, "segment_%03d.ts"),
			masterM3u8Path,
		)

		if err := cmd.Run(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "FFmpeg ပြောင်းလဲခြင်း မအောင်မြင်ပါ"})
			return
		}

		// ၄။ ရလာတဲ့ HLS ဖိုင်တွေ အားလုံးကို Cloudflare R2 ဆီ လှမ်းပို့မယ်
		log.Println("⏳ Cloudflare R2 သို့ Upload တင်နေပါသည်...")
		files, _ := os.ReadDir(tempDir)
		for _, f := range files {
			// 🌟 .key ဖိုင်ကိုပါ ExoPlayer ဖွင့်နိုင်ရန် R2 သို့ တင်ပေးရပါမည်
			if strings.HasSuffix(f.Name(), ".m3u8") || strings.HasSuffix(f.Name(), ".ts") || strings.HasSuffix(f.Name(), ".key") {
				err := uploadFileToR2(filepath.Join(tempDir, f.Name()), videoID+"/"+f.Name())
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "R2 သို့ Upload တင်ရာတွင် ကျရှုံးပါသည်: " + err.Error()})
					return
				}
			}
		}

		// ၅။ အောင်မြင်ကြောင်းနဲ့ ကြည့်ရမယ့် M3U8 Link ကို ပြန်ပို့ပေးမယ်
		publicUrl := fmt.Sprintf("https://pub-d56c2df021154436960a29402e69ff18.r2.dev/%s/master.m3u8", videoID)

		// 🌟 ၆။ Database (Supabase) ထဲသို့ ဗီဒီယို မှတ်တမ်းအသစ် သိမ်းဆည်းခြင်း
		newVideo := Video{
			Title:        title,
			Description:  description,
			VideoUrl:     publicUrl,
			ThumbnailUrl: "https://images.unsplash.com/photo-1611162617474-5b21e879e113", // ယာယီ ပုံ
			// Creator ID အစစ် မရှိသေးသဖြင့် UUID အလွတ်တစ်ခု ယာယီ ထည့်ထားမည်
			CreatorID: "00000000-0000-0000-0000-000000000000",
			Views:     0,
			IsPremium: false,
		}

		if err := db.Create(&newVideo).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database သို့ သိမ်းဆည်း၍မရပါ: " + err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Upload နှင့် Database သိမ်းဆည်းမှု အောင်မြင်ပါသည်",
			"video":   newVideo,
		})
	})

	r.Run("0.0.0.0:8080")
}

// R2 ဆီကို ဖိုင်တစ်ခုချင်းစီ ပို့ပေးမယ့် Function
// 🌟 Error ကို main ဆီ ပြန်ပို့ပေးမယ့် Function အဖြစ် ပြောင်းလိုက်ပါသည်
func uploadFileToR2(localFilePath string, r2Key string) error {
	file, err := os.Open(localFilePath)
	if err != nil {
		log.Println("ဖိုင် ဖွင့်မရပါ:", err)
		return err
	}
	defer file.Close()

	contentType := "video/mp2t"
	if strings.HasSuffix(localFilePath, ".m3u8") {
		contentType = "application/vnd.apple.mpegurl"
	}

	_, err = s3Client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:      aws.String(R2BucketName),
		Key:         aws.String(r2Key),
		Body:        file,
		ContentType: aws.String(contentType),
	})

	if err != nil {
		log.Println("R2 သို့ Upload လုပ်ရာတွင် Error ဖြစ်ပါသည်:", err)
		return err
	}
	return nil
}

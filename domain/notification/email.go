package notification

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"

	"github.com/dzuura/satu-lemari/domain/config"
	"github.com/google/uuid"
)

// EmailService handles email notifications
type EmailService struct {
	config *config.Config
}

// NewEmailService creates a new email service
func NewEmailService(cfg *config.Config) *EmailService {
	return &EmailService{
		config: cfg,
	}
}

// SendEmail sends an email notification
func (e *EmailService) SendEmail(ctx context.Context, userID uuid.UUID, notification *Notification) error {
	// Get user email (this would typically come from user service)
	userEmail, err := e.getUserEmail(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user email: %v", err)
	}

	// Create email content
	subject := notification.Title
	body := e.createEmailBody(notification)

	// Send email
	err = e.sendSMTPEmail(userEmail, subject, body)
	if err != nil {
		return fmt.Errorf("failed to send email: %v", err)
	}

	return nil
}

// SendBulkEmail sends emails to multiple users
func (e *EmailService) SendBulkEmail(ctx context.Context, userIDs []uuid.UUID, subject, body string) error {
	for _, userID := range userIDs {
		userEmail, err := e.getUserEmail(ctx, userID)
		if err != nil {
			continue // Skip this user
		}

		err = e.sendSMTPEmail(userEmail, subject, body)
		if err != nil {
			// Log error but continue with other users
			fmt.Printf("Failed to send email to %s: %v\n", userEmail, err)
		}
	}

	return nil
}

// createEmailBody creates the email body with HTML formatting
func (e *EmailService) createEmailBody(notification *Notification) string {
	htmlTemplate := `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>SatuLemari Notification</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            line-height: 1.6;
            color: #333;
            max-width: 600px;
            margin: 0 auto;
            padding: 20px;
        }
        .header {
            background-color: #4CAF50;
            color: white;
            padding: 20px;
            text-align: center;
            border-radius: 5px 5px 0 0;
        }
        .content {
            background-color: #f9f9f9;
            padding: 20px;
            border-radius: 0 0 5px 5px;
        }
        .message {
            background-color: white;
            padding: 15px;
            border-radius: 5px;
            margin: 15px 0;
            border-left: 4px solid #4CAF50;
        }
        .footer {
            text-align: center;
            margin-top: 20px;
            color: #666;
            font-size: 12px;
        }
        .button {
            display: inline-block;
            background-color: #4CAF50;
            color: white;
            padding: 10px 20px;
            text-decoration: none;
            border-radius: 5px;
            margin: 10px 0;
        }
        .priority-high {
            border-left-color: #f44336;
        }
        .priority-normal {
            border-left-color: #2196F3;
        }
        .priority-low {
            border-left-color: #FF9800;
        }
    </style>
</head>
<body>
    <div class="header">
        <h1>SatuLemari</h1>
        <p>Platform Donasi dan Rental Pakaian</p>
    </div>
    
    <div class="content">
        <div class="message priority-%s">
            <h2>%s</h2>
            <p>%s</p>
            <p><strong>Waktu:</strong> %s</p>
        </div>
        
        <div style="text-align: center; margin: 20px 0;">
            <a href="https://satu-lemari.vercel.app" class="button">Buka Aplikasi</a>
        </div>
        
        <div style="background-color: #e8f5e8; padding: 15px; border-radius: 5px; margin: 15px 0;">
            <h3>Mengapa Anda menerima email ini?</h3>
            <p>Email ini dikirim karena ada aktivitas penting terkait akun SatuLemari Anda. 
            Jika Anda tidak merasa melakukan aktivitas ini, silakan hubungi tim support kami.</p>
        </div>
    </div>
    
    <div class="footer">
        <p>&copy; 2024 SatuLemari. Semua hak cipta dilindungi.</p>
        <p>Email ini dikirim secara otomatis, mohon tidak membalas email ini.</p>
        <p>Untuk bantuan, hubungi: support@satulemari.com</p>
    </div>
</body>
</html>`

	priority := strings.ToLower(notification.Priority)
	formattedTime := notification.CreatedAt.Format("02 Januari 2006 15:04 WIB")

	return fmt.Sprintf(htmlTemplate,
		priority,
		notification.Title,
		notification.Message,
		formattedTime,
	)
}

// sendSMTPEmail sends email via SMTP
func (e *EmailService) sendSMTPEmail(to, subject, body string) error {
	// Check if email configuration is available
	if e.config.SMTPHost == "" || e.config.SMTPUsername == "" || e.config.SMTPPassword == "" {
		return fmt.Errorf("email configuration not available")
	}

	// Create SMTP authentication
	auth := smtp.PlainAuth("", e.config.SMTPUsername, e.config.SMTPPassword, e.config.SMTPHost)

	// Create email headers
	headers := make(map[string]string)
	headers["From"] = e.config.SMTPUsername
	headers["To"] = to
	headers["Subject"] = subject
	headers["MIME-Version"] = "1.0"
	headers["Content-Type"] = "text/html; charset=UTF-8"

	// Build email message
	message := ""
	for key, value := range headers {
		message += fmt.Sprintf("%s: %s\r\n", key, value)
	}
	message += "\r\n" + body

	// Send email
	addr := fmt.Sprintf("%s:%d", e.config.SMTPHost, e.config.SMTPPort)
	err := smtp.SendMail(addr, auth, e.config.SMTPUsername, []string{to}, []byte(message))
	if err != nil {
		return fmt.Errorf("failed to send email via SMTP: %v", err)
	}

	return nil
}

// getUserEmail gets user email from user service
// This is a placeholder - in real implementation, you would get this from user service
func (e *EmailService) getUserEmail(ctx context.Context, userID uuid.UUID) (string, error) {
	// TODO: Implement user service integration
	// For now, return a placeholder email
	// In real implementation, you would use ctx for timeout/cancellation
	// and userID to query the user service
	_ = ctx    // Suppress unused parameter warning
	_ = userID // Suppress unused parameter warning
	return "user@example.com", nil
}

// SendWelcomeEmail sends welcome email to new users
func (e *EmailService) SendWelcomeEmail(ctx context.Context, userID uuid.UUID, username string) error {
	userEmail, err := e.getUserEmail(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user email: %v", err)
	}

	subject := "Selamat Datang di SatuLemari!"
	body := e.createWelcomeEmailBody(username)

	return e.sendSMTPEmail(userEmail, subject, body)
}

// createWelcomeEmailBody creates welcome email body
func (e *EmailService) createWelcomeEmailBody(username string) string {
	htmlTemplate := `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Selamat Datang di SatuLemari</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            line-height: 1.6;
            color: #333;
            max-width: 600px;
            margin: 0 auto;
            padding: 20px;
        }
        .header {
            background-color: #4CAF50;
            color: white;
            padding: 20px;
            text-align: center;
            border-radius: 5px 5px 0 0;
        }
        .content {
            background-color: #f9f9f9;
            padding: 20px;
            border-radius: 0 0 5px 5px;
        }
        .welcome-message {
            background-color: white;
            padding: 20px;
            border-radius: 5px;
            margin: 15px 0;
            border-left: 4px solid #4CAF50;
        }
        .features {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 15px;
            margin: 20px 0;
        }
        .feature {
            background-color: white;
            padding: 15px;
            border-radius: 5px;
            text-align: center;
        }
        .feature-icon {
            font-size: 24px;
            margin-bottom: 10px;
        }
        .button {
            display: inline-block;
            background-color: #4CAF50;
            color: white;
            padding: 12px 24px;
            text-decoration: none;
            border-radius: 5px;
            margin: 10px 0;
            font-weight: bold;
        }
        .footer {
            text-align: center;
            margin-top: 20px;
            color: #666;
            font-size: 12px;
        }
    </style>
</head>
<body>
    <div class="header">
        <h1>🎉 Selamat Datang di SatuLemari!</h1>
        <p>Platform Donasi dan Rental Pakaian Terpercaya</p>
    </div>
    
    <div class="content">
        <div class="welcome-message">
            <h2>Halo %s! 👋</h2>
            <p>Terima kasih telah bergabung dengan komunitas SatuLemari. Kami senang Anda memilih platform kami untuk berbagi dan mendapatkan pakaian berkualitas.</p>
        </div>
        
        <h3>✨ Apa yang bisa Anda lakukan di SatuLemari?</h3>
        
        <div class="features">
            <div class="feature">
                <div class="feature-icon">🎁</div>
                <h4>Donasi Pakaian</h4>
                <p>Berikan pakaian layak pakai kepada yang membutuhkan</p>
            </div>
            <div class="feature">
                <div class="feature-icon">👕</div>
                <h4>Sewa Pakaian</h4>
                <p>Sewa pakaian untuk acara khusus dengan harga terjangkau</p>
            </div>
            <div class="feature">
                <div class="feature-icon">🤝</div>
                <h4>Komunitas</h4>
                <p>Bergabung dengan komunitas peduli lingkungan</p>
            </div>
            <div class="feature">
                <div class="feature-icon">🌱</div>
                <h4>Ramah Lingkungan</h4>
                <p>Berkontribusi pada kelestarian lingkungan</p>
            </div>
        </div>
        
        <div style="text-align: center; margin: 30px 0;">
            <a href="https://satu-lemari.vercel.app" class="button">Mulai Jelajahi SatuLemari</a>
        </div>
        
        <div style="background-color: #e8f5e8; padding: 20px; border-radius: 5px; margin: 20px 0;">
            <h3>🚀 Langkah Selanjutnya</h3>
            <ol>
                <li><strong>Lengkapi Profil:</strong> Tambahkan foto dan informasi kontak Anda</li>
                <li><strong>Jelajahi Katalog:</strong> Lihat pakaian yang tersedia untuk donasi atau sewa</li>
                <li><strong>Ajukan Permintaan:</strong> Pilih pakaian yang Anda butuhkan</li>
                <li><strong>Ikuti Panduan:</strong> Ikuti petunjuk pengambilan atau pengiriman</li>
            </ol>
        </div>
    </div>
    
    <div class="footer">
        <p>&copy; 2024 SatuLemari. Semua hak cipta dilindungi.</p>
        <p>Untuk bantuan, hubungi: support@satulemari.com</p>
        <p>Ikuti kami di: Instagram @satulemari | Twitter @satulemari</p>
    </div>
</body>
</html>`

	return fmt.Sprintf(htmlTemplate, username)
}

// SendPasswordResetEmail sends password reset email
func (e *EmailService) SendPasswordResetEmail(ctx context.Context, userID uuid.UUID, resetToken string) error {
	userEmail, err := e.getUserEmail(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user email: %v", err)
	}

	subject := "Reset Password SatuLemari"
	body := e.createPasswordResetEmailBody(resetToken)

	return e.sendSMTPEmail(userEmail, subject, body)
}

// createPasswordResetEmailBody creates password reset email body
func (e *EmailService) createPasswordResetEmailBody(resetToken string) string {
	resetURL := fmt.Sprintf("https://satu-lemari.vercel.app/reset-password?token=%s", resetToken)

	htmlTemplate := `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Reset Password SatuLemari</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            line-height: 1.6;
            color: #333;
            max-width: 600px;
            margin: 0 auto;
            padding: 20px;
        }
        .header {
            background-color: #f44336;
            color: white;
            padding: 20px;
            text-align: center;
            border-radius: 5px 5px 0 0;
        }
        .content {
            background-color: #f9f9f9;
            padding: 20px;
            border-radius: 0 0 5px 5px;
        }
        .reset-message {
            background-color: white;
            padding: 20px;
            border-radius: 5px;
            margin: 15px 0;
            border-left: 4px solid #f44336;
        }
        .button {
            display: inline-block;
            background-color: #f44336;
            color: white;
            padding: 12px 24px;
            text-decoration: none;
            border-radius: 5px;
            margin: 10px 0;
            font-weight: bold;
        }
        .warning {
            background-color: #fff3cd;
            border: 1px solid #ffeaa7;
            color: #856404;
            padding: 15px;
            border-radius: 5px;
            margin: 15px 0;
        }
        .footer {
            text-align: center;
            margin-top: 20px;
            color: #666;
            font-size: 12px;
        }
    </style>
</head>
<body>
    <div class="header">
        <h1>🔐 Reset Password SatuLemari</h1>
        <p>Permintaan reset password Anda</p>
    </div>
    
    <div class="content">
        <div class="reset-message">
            <h2>Reset Password</h2>
            <p>Anda telah meminta untuk mereset password akun SatuLemari Anda. Klik tombol di bawah untuk melanjutkan:</p>
            
            <div style="text-align: center; margin: 20px 0;">
                <a href="%s" class="button">Reset Password</a>
            </div>
            
            <p>Atau copy link berikut ke browser Anda:</p>
            <p style="word-break: break-all; background-color: #f8f9fa; padding: 10px; border-radius: 3px; font-family: monospace;">%s</p>
        </div>
        
        <div class="warning">
            <h3>⚠️ Penting!</h3>
            <ul>
                <li>Link ini hanya berlaku selama 1 jam</li>
                <li>Jangan bagikan link ini kepada siapapun</li>
                <li>Jika Anda tidak meminta reset password, abaikan email ini</li>
            </ul>
        </div>
    </div>
    
    <div class="footer">
        <p>&copy; 2024 SatuLemari. Semua hak cipta dilindungi.</p>
        <p>Untuk bantuan, hubungi: support@satulemari.com</p>
    </div>
</body>
</html>`

	return fmt.Sprintf(htmlTemplate, resetURL, resetURL)
}

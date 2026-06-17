package email

import (
	"crypto/tls"
	"log"
	"net"
	"net/smtp"
	"time"

	"github.com/konghang/ember/backend/internal/common/upstream"
	configpkg "github.com/konghang/ember/backend/internal/config"
	"gopkg.in/gomail.v2"
)

// sendEmail 通过 SMTP 发送邮件（使用 gomail 生成 MIME，并设置全链路超时）
func (s *EmailService) sendEmail(to, subject, body string) error {
	if s.sendEmailFunc != nil {
		return s.sendEmailFunc(to, subject, body)
	}
	s.refreshConfig()
	m := gomail.NewMessage()
	m.SetHeader("From", s.from)
	m.SetHeader("To", to)
	m.SetHeader("Subject", subject)
	m.SetBody("text/plain", body)

	addr := net.JoinHostPort(s.host, s.port)
	conn, err := net.DialTimeout("tcp", addr, smtpTimeout)
	if err != nil {
		return upstream.SafeUpstreamError(err, "smtp")
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(smtpTimeout))

	client, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return upstream.SafeUpstreamError(err, "smtp")
	}
	defer client.Close()

	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: s.host}); err != nil {
			return upstream.SafeUpstreamError(err, "smtp")
		}
	}

	auth := smtp.PlainAuth("", s.username, s.password, s.host)
	if err := client.Auth(auth); err != nil {
		return upstream.SafeUpstreamError(err, "smtp")
	}

	if err := client.Mail(s.fromAddress); err != nil {
		return upstream.SafeUpstreamError(err, "smtp")
	}

	if err := client.Rcpt(to); err != nil {
		return upstream.SafeUpstreamError(err, "smtp")
	}

	w, err := client.Data()
	if err != nil {
		return upstream.SafeUpstreamError(err, "smtp")
	}

	if _, err := m.WriteTo(w); err != nil {
		w.Close()
		return upstream.SafeUpstreamError(err, "smtp")
	}

	if err := w.Close(); err != nil {
		return upstream.SafeUpstreamError(err, "smtp")
	}

	if err := client.Quit(); err != nil {
		log.Printf("SMTP QUIT 失败 [%s]: %v", to, err)
	}

	return nil
}

func (s *EmailService) TestConnection() error {
	s.refreshConfig()
	if !s.IsConfigured() {
		return ErrEmailNotConfigured
	}
	return configpkg.TestSMTPDial(s.host, s.port)
}

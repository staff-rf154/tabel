package jobs

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"tabel/internal/repo"
)

const defaultInterval = time.Hour

func Run(ctx context.Context, pool *pgxpool.Pool) {
	runOnce(ctx, pool)
	ticker := time.NewTicker(defaultInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runOnce(ctx, pool)
		}
	}
}

func runOnce(ctx context.Context, pool *pgxpool.Pool) {
	rp := repo.New(pool)

	for i := 0; i < 4; i++ {
		date := time.Now().AddDate(0, i, 0).Format("2006-01-02")
		if err := rp.EnsureMarksPartition(ctx, date); err != nil {
			log.Printf("планировщик: партиция отметок на %s: %v", date, err)
		}
	}

	if expired, err := rp.ExpirePaperTokens(ctx); err != nil {
		log.Printf("планировщик: истёкшие бумажные QR: %v", err)
	} else if expired > 0 {
		log.Printf("планировщик: помечено истёкшими бумажных QR: %d", expired)
	}

	if expired, err := rp.ExpireDelegations(ctx); err != nil {
		log.Printf("планировщик: завершение делегирований: %v", err)
	} else if expired > 0 {
		log.Printf("планировщик: завершено делегирований: %d", expired)
	}

	if tokens, err := rp.ExpiringPaperTokens(ctx, 5); err != nil {
		log.Printf("планировщик: QR с истечением: %v", err)
	} else {
		for _, token := range tokens {
			marker := fmt.Sprintf("%s|%s", token.ID, token.ValidUntil)
			exists, err := rp.NotificationExistsForPayload(ctx, "qr.expiring", "qr_token", marker)
			if err != nil || exists {
				continue
			}
			body := "Срок действия бумажного QR-токена истекает в течение 5 дней"
			_ = rp.NotifyAdmins(ctx, "qr.expiring", body, &body, map[string]any{
				"qr_token":        marker,
				"employee_id":     token.EmployeeID,
				"employee_number": token.EmployeeNumber,
				"full_name":       token.FullName,
				"valid_until":     token.ValidUntil,
			})
		}
	}

	if clients, err := rp.OAuthClientsDueForRotation(ctx, 5); err != nil {
		log.Printf("планировщик: ротация секретов OAuth: %v", err)
	} else {
		for _, client := range clients {
			rotated := ""
			if client.SecretRotatedAt != nil {
				rotated = client.SecretRotatedAt.UTC().Format(time.RFC3339)
			}
			nextRotation := ""
			if client.NextRotationAt != nil {
				nextRotation = client.NextRotationAt.UTC().Format(time.RFC3339)
			}
			name := ""
			if client.Name != nil {
				name = *client.Name
			}
			marker := fmt.Sprintf("%s|%s", client.ID, rotated)
			exists, err := rp.NotificationExistsForPayload(ctx, "oauth.secret_rotation", "oauth_client", marker)
			if err != nil || exists {
				continue
			}
			body := "Требуется ротация секрета OAuth-клиента (не позднее чем через 5 дней)"
			_ = rp.NotifyAdmins(ctx, "oauth.secret_rotation", body, &body, map[string]any{
				"oauth_client":     marker,
				"client_id":        client.ClientID,
				"name":             name,
				"next_rotation_at": nextRotation,
			})
		}
	}
}

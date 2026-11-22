// Command aic-recette-inject livre un webhook issue_comment signé au
// récepteur du coordinateur en boucle locale (recette M-AIC-02 T02) : elle
// construit le payload, le signe en HMAC-SHA256 et le poste sur l'adresse
// d'écoute. Aucune écriture GitHub réelle (AIC_GITHUB_MODE=off), aucune
// exposition réseau : l'adresse cible est imposée en boucle locale.
package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "adresse du récepteur (boucle locale uniquement)")
	secret := flag.String("secret", "", "secret HMAC (AIC_WEBHOOK_SECRET)")
	issue := flag.Int("issue", 0, "numéro d'Issue (mission AIC-<numéro>)")
	sender := flag.String("sender", "dbouzon", "auteur authentifié du commentaire")
	body := flag.String("body", "/aic start", "corps du commentaire (commande /aic)")
	delivery := flag.String("delivery", "", "identifiant de livraison (défaut : horodatage)")
	title := flag.String("title", "Mission de recette M-AIC-02", "titre de l'Issue")
	flag.Parse()
	if err := run(*addr, *secret, *issue, *sender, *body, *delivery, *title); err != nil {
		fmt.Fprintln(os.Stderr, "erreur:", err)
		os.Exit(1)
	}
}

func run(addr, secret string, issue int, sender, body, delivery, title string) error {
	if secret == "" {
		return fmt.Errorf("secret requis (AIC_WEBHOOK_SECRET)")
	}
	if issue <= 0 {
		return fmt.Errorf("numéro d'Issue requis")
	}
	if host, _, err := net.SplitHostPort(addr); err != nil {
		return fmt.Errorf("adresse invalide : %w", err)
	} else if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("seule la boucle locale est autorisée (recette sans exposition), reçu : %s", addr)
	}
	if delivery == "" {
		delivery = "recette-" + time.Now().UTC().Format("20060102T150405.000000000")
	}
	raw, err := buildPayload(issue, sender, body, title)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, "http://"+addr+"/webhook", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "issue_comment")
	req.Header.Set("X-GitHub-Delivery", delivery)
	req.Header.Set("X-Hub-Signature-256", "sha256="+signHex(secret, raw))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	fmt.Printf("livraison %s : statut HTTP %d\n", delivery, resp.StatusCode)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("statut HTTP inattendu : %d", resp.StatusCode)
	}
	return nil
}

// buildPayload construit le payload minimal exploité par le coordinateur.
func buildPayload(issue int, sender, body, title string) ([]byte, error) {
	p := map[string]any{
		"action": "created",
		"issue":  map[string]any{"number": issue, "title": title},
		"comment": map[string]any{
			"body": body,
			"user": map[string]any{"login": sender},
		},
		"sender": map[string]any{"login": sender},
	}
	return json.Marshal(p)
}

// signHex calcule la signature HMAC-SHA256 du payload.
func signHex(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return strings.ToLower(hex.EncodeToString(mac.Sum(nil)))
}

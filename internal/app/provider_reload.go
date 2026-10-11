package app

import (
	"context"
	"fmt"
	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/hotstate"
	"veloxmesh/internal/providers"
)

func (a *App) ReloadProviders(ctx context.Context, repo controlstate.Repository, cipher controlstate.SecretCipher) error {
	records, err := controlstate.LoadActiveProviderRecords(ctx, repo.Providers())
	if err != nil {
		return fmt.Errorf("failed to load active provider records: %w", err)
	}

	var rCfg *controlstate.RoutingConfig
	if repo.Routing() != nil {
		rCfg, err = repo.Routing().Get(ctx)
		if err != nil && err != controlstate.ErrRoutingConfigNotFound {
			return fmt.Errorf("failed to load routing config: %w", err)
		}
		// If ErrRoutingConfigNotFound, rCfg stays nil, meaning no routing config applied yet.
	}

	secrets := make(map[string]string)
	for _, r := range records {
		if !r.Enabled {
			continue
		}
		if !r.Secret.SecretConfigured {
			return fmt.Errorf("provider %s has no secret configured", r.ID)
		}
		ciphertext, nonce, keyID, err := repo.Providers().GetEncryptedSecret(ctx, r.ID)
		if err != nil {
			return fmt.Errorf("failed to get encrypted secret for %s: %w", r.ID, err)
		}
		decrypted, err := cipher.DecryptProviderSecret(&controlstate.EncryptedSecret{
			Ciphertext: ciphertext,
			Nonce:      nonce,
			KeyID:      keyID,
		})
		if err != nil {
			return fmt.Errorf("failed to decrypt secret for %s: %w", r.ID, err)
		}
		secrets[r.ID] = string(decrypted)
	}

	var combos []providers.Combo
	if repo.Combos() != nil {
		enabled := true
		comboRecords, err := repo.Combos().List(ctx, controlstate.ComboFilter{Enabled: &enabled})
		if err != nil {
			return fmt.Errorf("failed to load combos: %w", err)
		}
		for _, rec := range comboRecords {
			combos = append(combos, providers.Combo{
				ID:       rec.ID,
				Name:     rec.Name,
				Strategy: rec.Strategy,
				Members:  rec.Members,
				Judge:    rec.Judge,
			})
		}
	}

	var semRules *controlstate.SemanticRuleSnapshot
	if repo.SemanticRules() != nil {
		global, err := repo.SemanticRules().GetGlobalDefaults(ctx)
		if err != nil {
			a.Logger.Error("failed to load global semantic rules", "error", err)
		} else {
			users, err := repo.SemanticRules().ListUserConfigs(ctx)
			if err != nil {
				a.Logger.Error("failed to load user semantic rules", "error", err)
			} else {
				semRules = &controlstate.SemanticRuleSnapshot{
					Global: global,
					Users:  users,
				}
			}
		}
	}

	rates := make(map[string]float64)
	if repo.Rates() != nil {
		for _, r := range records {
			if !r.Enabled {
				continue
			}
			for _, m := range r.Models {
				if rate, err := repo.Rates().Get(ctx, r.ID, m); err == nil && rate != nil {
					rates[r.ID+":"+m] = float64(rate.InputCreditRate + rate.OutputCreditRate)
				}
			}
		}
	}

	return a.RuntimeProviderManager.ActivateDurable(ctx, records, secrets, rCfg, combos, semRules, rates, nil)
}

func (a *App) StartConfigChangeSubscriber(ctx context.Context, repo controlstate.Repository, cipher controlstate.SecretCipher) error {
	sub, err := a.HotState.SubscribeConfigChanges(ctx)
	if err != nil {
		return fmt.Errorf("failed to subscribe to config changes: %w", err)
	}

	a.Logger.Info("starting config change subscriber")

	go func() {
		defer sub.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-sub.Channel():
				if msg == nil {
					return
				}
				a.Logger.Info("received config change notification", "type", msg.Type, "target_id", msg.TargetID, "action", msg.Action, "revision", msg.Revision)

				switch msg.Type {
				case hotstate.EventProvider, hotstate.EventCombo, hotstate.EventRouting:
					if err := a.ReloadProviders(ctx, repo, cipher); err != nil {
						a.Logger.Error("failed to reload providers on config change", "error", err)
					}
				case hotstate.EventSemanticRules:
					if err := a.ReloadSemanticRules(ctx, repo); err != nil {
						a.Logger.Error("failed to reload semantic rules on config change", "error", err)
					}
				case hotstate.EventAPIKey:
					// Invalidate hot cache for API key
					if err := a.HotState.Delete(ctx, hotstate.NamespacedKey(a.Config.RedisNamespace, "auth", msg.TargetID)); err != nil {
						a.Logger.Error("failed to invalidate api key cache", "error", err)
					}
				case hotstate.EventLimitRule:
					a.Logger.Info("limit rule changed, no in-memory reload needed")
				case hotstate.EventVectorPolicy:
					a.Logger.Info("vector policy changed, requires restart for now")
				default:
					a.Logger.Warn("unknown event type, falling back to full reload", "type", msg.Type)
					if err := a.ReloadProviders(ctx, repo, cipher); err != nil {
						a.Logger.Error("failed to reload providers on unknown config change", "error", err)
					}
				}
			}
		}
	}()

	return nil
}

func (a *App) ReloadSemanticRules(ctx context.Context, repo controlstate.Repository) error {
	if repo.SemanticRules() == nil {
		return nil
	}

	global, err := repo.SemanticRules().GetGlobalDefaults(ctx)
	if err != nil {
		return fmt.Errorf("failed to load global semantic rules: %w", err)
	}

	users, err := repo.SemanticRules().ListUserConfigs(ctx)
	if err != nil {
		return fmt.Errorf("failed to load user semantic rules: %w", err)
	}

	semRules := &controlstate.SemanticRuleSnapshot{
		Global: global,
		Users:  users,
	}

	a.RuntimeProviderManager.UpdateSemanticRules(semRules)
	return nil
}

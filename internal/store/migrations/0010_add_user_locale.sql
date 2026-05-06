-- Per-user UI locale preference. 'zh-TW' default keeps existing rows valid;
-- 'en' is the only other recognised value at the moment. Set/read by the
-- TUI's screen_locale_settings via internal/store.UserRepo.SetLocale and
-- internal/i18n.Normalize. Adding a new locale means adding a row to
-- internal/i18n's locale tables — no schema migration needed.
ALTER TABLE users ADD COLUMN locale TEXT NOT NULL DEFAULT 'zh-TW';

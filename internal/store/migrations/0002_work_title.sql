-- 0002：作品重命名。title_override 为空时沿用最新版本 work.yaml 的标题。
ALTER TABLE works ADD COLUMN title_override TEXT;

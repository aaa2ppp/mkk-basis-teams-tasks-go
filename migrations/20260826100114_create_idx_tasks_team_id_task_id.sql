-- +goose Up
ALTER TABLE tasks ADD INDEX idx_tasks_team_id_task_id (team_id, id);    

-- +goose Down
-- MariaDB автоматически "повесила" внешний ключ fk_tasks_team_id_teams на наш 
-- новый индекс idx_tasks_team_id_task_id, чтобы не плодить дублирующие индексы.
-- Поэтому просто удалить индекс нельзя (ошибка 1553). Приходится временно снять FK.
ALTER TABLE tasks 
    DROP FOREIGN KEY fk_tasks_team_id_teams,
    DROP INDEX idx_tasks_team_id_task_id,
    ADD CONSTRAINT fk_tasks_team_id_teams FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE;
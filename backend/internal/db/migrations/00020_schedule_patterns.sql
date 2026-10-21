-- +goose Up
ALTER TABLE schedules DROP CONSTRAINT IF EXISTS schedules_pattern_type_check;
ALTER TABLE schedules ADD CONSTRAINT schedules_pattern_type_check
    CHECK (pattern_type IN ('5/2', '2/2', '4/2', '1/2', 'custom'));

ALTER TABLE schedule_templates DROP CONSTRAINT IF EXISTS schedule_templates_pattern_type_check;
ALTER TABLE schedule_templates ADD CONSTRAINT schedule_templates_pattern_type_check
    CHECK (pattern_type IN ('5/2', '2/2', '4/2', '1/2', 'custom'));

-- +goose Down
UPDATE schedules SET pattern_type = 'custom' WHERE pattern_type IN ('4/2', '1/2');
UPDATE schedule_templates SET pattern_type = 'custom' WHERE pattern_type IN ('4/2', '1/2');

ALTER TABLE schedules DROP CONSTRAINT IF EXISTS schedules_pattern_type_check;
ALTER TABLE schedules ADD CONSTRAINT schedules_pattern_type_check
    CHECK (pattern_type IN ('5/2', '2/2', 'custom'));

ALTER TABLE schedule_templates DROP CONSTRAINT IF EXISTS schedule_templates_pattern_type_check;
ALTER TABLE schedule_templates ADD CONSTRAINT schedule_templates_pattern_type_check
    CHECK (pattern_type IN ('5/2', '2/2', 'custom'));

-- +goose Up
UPDATE material_kinds SET label = 'Лабораторная' WHERE code = 'lab';
UPDATE material_kinds SET label = 'Туториал' WHERE code = 'tutorial';

-- +goose Down
UPDATE material_kinds SET label = 'Лабораторная' WHERE code = 'lab';
UPDATE material_kinds SET label = 'Практика' WHERE code = 'tutorial';

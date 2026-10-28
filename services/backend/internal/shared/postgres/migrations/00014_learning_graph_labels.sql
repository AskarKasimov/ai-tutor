-- +goose Up
CREATE TABLE learning_graph_labels (
    cache_key text PRIMARY KEY,
    display_name text NOT NULL CHECK (length(display_name) BETWEEN 1 AND 80)
);

-- +goose Down
DROP TABLE learning_graph_labels;

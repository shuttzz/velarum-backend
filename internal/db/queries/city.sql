-- name: CreateCity :one
INSERT INTO cities (
    world_id, player_id, name, coord_x, coord_y, era,
    matter_stored, energy_stored, knowledge_stored,
    matter_rate, energy_rate, knowledge_rate,
    matter_cap, energy_cap, knowledge_cap,
    resources_updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9,
    $10, $11, $12,
    $13, $14, $15,
    $16
)
RETURNING *;

-- name: GetCity :one
SELECT * FROM cities WHERE id = $1;

-- name: UpdateCityResources :exec
UPDATE cities SET
    matter_stored = $2, energy_stored = $3, knowledge_stored = $4,
    matter_rate = $5, energy_rate = $6, knowledge_rate = $7,
    resources_updated_at = $8
WHERE id = $1;

-- name: AddCityBuilding :one
INSERT INTO city_buildings (city_id, slot_index, building_type, level)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListCityBuildings :many
SELECT * FROM city_buildings WHERE city_id = $1 ORDER BY slot_index;

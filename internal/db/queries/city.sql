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

-- name: GetCityForUpdate :one
SELECT * FROM cities WHERE id = $1 FOR UPDATE;

-- name: UpdateCityResources :exec
UPDATE cities SET
    matter_stored = $2, energy_stored = $3, knowledge_stored = $4,
    matter_rate = $5, energy_rate = $6, knowledge_rate = $7,
    resources_updated_at = $8
WHERE id = $1;

-- name: InsertCityBuilding :one
INSERT INTO city_buildings (city_id, building_type, level, pos_x, pos_y)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListCityBuildings :many
SELECT * FROM city_buildings WHERE city_id = $1 ORDER BY pos_y, pos_x;

-- name: GetCityBuildingForUpdate :one
SELECT * FROM city_buildings WHERE id = $1 FOR UPDATE;

-- name: SetCityBuildingLevel :exec
UPDATE city_buildings SET level = $2 WHERE id = $1;

-- name: MoveCityBuilding :exec
UPDATE city_buildings SET pos_x = $2, pos_y = $3 WHERE id = $1;

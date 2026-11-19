CREATE TABLE categories (
 id UUID PRIMARY KEY,
 name TEXT NOT NULL CHECK (length(btrim(name)) > 0)
);

CREATE TABLE products (
 id UUID PRIMARY KEY,
 category_id UUID NOT NULL REFERENCES categories(id),
 name TEXT NOT NULL CHECK (length(btrim(name)) > 0),
 description TEXT NOT NULL,
 price_usd NUMERIC NOT NULL CHECK (price_usd > 0 AND price_usd < 'Infinity'::numeric),
 active BOOLEAN NOT NULL DEFAULT TRUE,
 image_url TEXT
);

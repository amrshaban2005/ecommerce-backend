-- Service databases and users, initialized when PostgreSQL creates an empty volume.
-- Local development passwords; existing roles and databases are left unchanged.
SELECT format('CREATE ROLE %I LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE PASSWORD %L', service.name, service.password)
FROM (VALUES
    ('auth', 'auth_local_only'),
    ('product', 'product_local_only'),
    ('cart', 'cart_local_only'),
    ('inventory', 'inventory_local_only'),
    ('order', 'order_local_only'),
    ('payment', 'payment_local_only')
) AS service(name, password)
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = service.name)
\gexec

SELECT format('CREATE DATABASE %I OWNER %I', service.database, service.owner)
FROM (VALUES
    ('auth_db', 'auth'),
    ('product_db', 'product'),
    ('cart_db', 'cart'),
    ('inventory_db', 'inventory'),
    ('order_db', 'order'),
    ('payment_db', 'payment')
) AS service(database, owner)
WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = service.database)
\gexec

REVOKE CONNECT ON DATABASE auth_db FROM PUBLIC;
GRANT CONNECT ON DATABASE auth_db TO auth;

REVOKE CONNECT ON DATABASE product_db FROM PUBLIC;
GRANT CONNECT ON DATABASE product_db TO product;

REVOKE CONNECT ON DATABASE cart_db FROM PUBLIC;
GRANT CONNECT ON DATABASE cart_db TO cart;

REVOKE CONNECT ON DATABASE inventory_db FROM PUBLIC;
GRANT CONNECT ON DATABASE inventory_db TO inventory;

REVOKE CONNECT ON DATABASE order_db FROM PUBLIC;
GRANT CONNECT ON DATABASE order_db TO "order";

REVOKE CONNECT ON DATABASE payment_db FROM PUBLIC;
GRANT CONNECT ON DATABASE payment_db TO payment;

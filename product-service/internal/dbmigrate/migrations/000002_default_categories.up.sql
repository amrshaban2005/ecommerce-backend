-- Default computer-shop categories. Preserve any existing rows with these IDs.
INSERT INTO categories (id, name) VALUES
    ('019a6000-0000-7000-8000-000000000001', 'Laptops'),
    ('019a6000-0000-7000-8000-000000000002', 'Desktops'),
    ('019a6000-0000-7000-8000-000000000003', 'Monitors'),
    ('019a6000-0000-7000-8000-000000000004', 'Components'),
    ('019a6000-0000-7000-8000-000000000005', 'Peripherals'),
    ('019a6000-0000-7000-8000-000000000006', 'Accessories')
ON CONFLICT (id) DO NOTHING;

create table if not exists product (
seller_id integer not null,
offer_id serial not null,
name varchar(100) not null,
price integer not null,
quantity integer not null,
available boolean not null,
constraint product_id primary key(seller_id, offer_id)
);


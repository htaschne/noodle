#!/bin/sh
set -eu

install -d -m 0700 -o www-data -g www-data /run/noodle-moodle
cd /var/www/html

# Check the database on each startup, independently of the data volume.
tables=$(php -d display_errors=stderr -r '
    $db = pg_connect("host=moodle-db dbname=moodle user=moodle password=" . getenv("DEMO_MOODLE_DB_PASSWORD"));
    $tables = pg_query($db, "SELECT count(*) FROM pg_tables WHERE schemaname = current_schema()");
    echo pg_fetch_result($tables, 0, 0);
')
if [ "$tables" = 0 ]; then
    runuser -u www-data -- php admin/cli/install_database.php --agree-license \
        --fullname='Noodle Moodle Demo' --shortname='Noodle Demo' \
        --adminemail=admin@example.test --adminpass="$DEMO_MOODLE_ADMIN_PASSWORD"
fi

runuser -u www-data -- php /usr/local/share/noodle-moodle/seed.php
exec apache2-foreground

FROM ghcr.io/moodlehq/moodle-php-apache:8.3-bookworm@sha256:1db7b6ed013fcb5268095b5e28c410ba9117d38d9b6a324c100c48fcbe69745e

# Moodle 4.5.16, pinned to the release tag's commit and tarball checksum.
ADD --checksum=sha256:217f3e94a5b6600b60d0470dbe57f47b845b31a0b97444f04f069788263d72c1 https://codeload.github.com/moodle/moodle/tar.gz/69ef83ad2948db19595106671f03a1429679c870 /tmp/moodle.tgz
RUN tar -xzf /tmp/moodle.tgz --strip-components=1 -C /var/www/html \
    && rm /tmp/moodle.tgz \
    && sed -i 's/^Listen 80$/Listen 18080/' /etc/apache2/ports.conf \
    && sed -i 's/\*:80>/*:18080>/' /etc/apache2/sites-available/000-default.conf \
    && printf 'max_input_vars=5000\n' > /usr/local/etc/php/conf.d/noodle-moodle.ini

COPY config.php /var/www/html/config.php
COPY seed.php /usr/local/share/noodle-moodle/seed.php
COPY --chmod=755 start.sh /usr/local/bin/noodle-moodle-start

CMD ["noodle-moodle-start"]

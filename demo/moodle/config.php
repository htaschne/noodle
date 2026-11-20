<?php

unset($CFG);
$CFG = new stdClass();
$CFG->dbtype = 'pgsql';
$CFG->dblibrary = 'native';
$CFG->dbhost = 'moodle-db';
$CFG->dbname = 'moodle';
$CFG->dbuser = 'moodle';
$CFG->dbpass = getenv('DEMO_MOODLE_DB_PASSWORD');
$CFG->prefix = 'mdl_';
$CFG->wwwroot = getenv('MOODLE_WWWROOT');
$CFG->dataroot = '/var/www/moodledata';
$CFG->admin = 'admin';
$CFG->directorypermissions = 02770;
$CFG->noemailever = true;

require_once(__DIR__ . '/lib/setup.php');

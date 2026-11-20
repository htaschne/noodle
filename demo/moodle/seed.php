<?php

define('CLI_SCRIPT', true);
require('/var/www/html/config.php');
require_once($CFG->dirroot . '/course/lib.php');
require_once($CFG->dirroot . '/course/modlib.php');
require_once($CFG->dirroot . '/mod/assign/locallib.php');
require_once($CFG->dirroot . '/user/lib.php');
require_once($CFG->dirroot . '/webservice/lib.php');

function demo_user(string $username, string $firstname, string $password): stdClass {
    global $CFG, $DB;
    $user = $DB->get_record('user', ['username' => $username, 'mnethostid' => $CFG->mnet_localhost_id]);
    if (!$user) {
        $id = user_create_user((object) [
            'username' => $username, 'firstname' => $firstname, 'lastname' => 'Example',
            'email' => "$username@example.test", 'auth' => 'manual', 'confirmed' => 1,
            'mnethostid' => $CFG->mnet_localhost_id, 'maildisplay' => 2, 'password' => $password,
        ]);
        $user = $DB->get_record('user', ['id' => $id], '*', MUST_EXIST);
    }
    return $user;
}

function demo_role(string $shortname, string $name, context $context, array $capabilities): int {
    global $DB;
    $roleid = $DB->get_field('role', 'id', ['shortname' => $shortname]);
    if (!$roleid) {
        $roleid = create_role($name, $shortname, 'Noodle local demonstration');
        set_role_contextlevels($roleid, [$context->contextlevel]);
    }
    foreach ($capabilities as $capability) {
        assign_capability($capability, CAP_ALLOW, $roleid, $context->id);
    }
    return $roleid;
}

function demo_assignment(stdClass $course, string $key, string $name, string $description, int $due): assign {
    global $DB;
    $module = $DB->get_field('modules', 'id', ['name' => 'assign'], MUST_EXIST);
    $cmid = $DB->get_field('course_modules', 'id', ['course' => $course->id, 'idnumber' => $key]);
    if (!$cmid) {
        $activity = add_moduleinfo((object) [
            'module' => $module, 'modulename' => 'assign', 'section' => 0, 'visible' => 1,
            'cmidnumber' => $key, 'name' => $name, 'intro' => $description, 'introformat' => FORMAT_PLAIN,
            'alwaysshowdescription' => 1, 'submissiondrafts' => 1, 'requiresubmissionstatement' => 0,
            'sendnotifications' => 0, 'sendlatenotifications' => 0, 'sendstudentnotifications' => 0,
            'duedate' => $due, 'cutoffdate' => 0, 'gradingduedate' => 0, 'allowsubmissionsfromdate' => 0,
            'grade' => 100, 'teamsubmission' => 0, 'requireallteammemberssubmit' => 0,
            'blindmarking' => 0, 'markingworkflow' => 0, 'markingallocation' => 0,
            'assignsubmission_onlinetext_enabled' => 1, 'assignsubmission_file_enabled' => 0,
        ], $course);
        $cmid = $activity->coursemodule;
    }
    return new assign(context_module::instance($cmid), null, $course);
}

function demo_submission(assign $assignment, stdClass $student, bool $submit): void {
    if ($assignment->get_user_submission($student->id, false)) {
        return;
    }
    \core\session\manager::set_user($student);
    $notices = [];
    $data = (object) ['onlinetext_editor' => ['text' => 'Fictitious coursework.', 'format' => FORMAT_PLAIN, 'itemid' => 0]];
    if (!$assignment->save_submission($data, $notices) || ($submit && !$assignment->submit_for_grading((object) [], $notices))) {
        throw new RuntimeException('Could not seed the demonstration submission: ' . implode(', ', $notices));
    }
    \core\session\manager::set_user(get_admin());
}

\core\session\manager::set_user(get_admin());
$transaction = $DB->start_delegated_transaction();
set_config('enablewebservices', 1);
set_config('webserviceprotocols', 'rest');

$course = $DB->get_record('course', ['shortname' => 'NOODLE-DEMO']);
if (!$course) {
    $course = create_course((object) [
        'category' => core_course_category::get_default()->id, 'fullname' => 'Noodle Demonstration Course',
        'shortname' => 'NOODLE-DEMO', 'format' => 'topics', 'visible' => 1, 'groupmode' => 0,
    ]);
}
$manual = enrol_get_plugin('manual');
$enrolment = $DB->get_record('enrol', ['courseid' => $course->id, 'enrol' => 'manual'], '*', MUST_EXIST);
$studentrole = $DB->get_field('role', 'id', ['shortname' => 'student'], MUST_EXIST);
$students = [];
foreach (['alex' => 'Alex', 'blair' => 'Blair', 'casey' => 'Casey'] as $username => $firstname) {
    $students[$username] = demo_user($username, $firstname, getenv('DEMO_MOODLE_STUDENT_PASSWORD'));
    $manual->enrol_user($enrolment, $students[$username]->id, $studentrole);
}

$monitor = demo_user('noodle_monitor', 'Noodle Monitor', bin2hex(random_bytes(24)) . '!aZ');
$system = context_system::instance();
role_assign(demo_role('noodle_rest', 'Noodle REST access', $system, ['webservice/rest:use']), $monitor->id, $system->id);
$monitorrole = demo_role('noodle_monitor', 'Noodle course monitor', context_course::instance($course->id), [
    'moodle/course:view', 'moodle/course:viewparticipants', 'moodle/course:enrolreview',
    'moodle/course:useremail', 'moodle/user:viewdetails', 'mod/assign:view', 'mod/assign:viewgrades',
]);
$manual->enrol_user($enrolment, $monitor->id, $monitorrole);

$webservices = new webservice();
$service = $DB->get_record('external_services', ['shortname' => 'noodle_monitoring']);
if (!$service) {
    $service = (object) ['name' => 'Noodle monitoring', 'shortname' => 'noodle_monitoring', 'enabled' => 1, 'restrictedusers' => 1];
    $service->id = $webservices->add_external_service($service);
}
foreach (['core_webservice_get_site_info', 'core_enrol_get_users_courses', 'mod_assign_get_assignments',
          'core_enrol_get_enrolled_users', 'mod_assign_get_submissions', 'mod_assign_get_submission_status'] as $function) {
    if (!$webservices->service_function_exists($function, $service->id)) {
        $webservices->add_external_function_to_service($function, $service->id);
    }
}
$authorization = ['externalserviceid' => $service->id, 'userid' => $monitor->id];
if (!$DB->record_exists('external_services_users', $authorization)) {
    $webservices->add_ws_authorised_user((object) $authorization);
}
$token = $DB->get_field('external_tokens', 'token', $authorization + ['tokentype' => EXTERNAL_TOKEN_PERMANENT]);
if (!$token) {
    $token = \core_external\util::generate_token(EXTERNAL_TOKEN_PERMANENT, $service, $monitor->id, $system, name: 'Noodle local demo');
}

$now = time();
$essay = demo_assignment($course, 'noodle-essay', 'Pending essay', 'Alex has never started, Blair has submitted, and Casey has a draft.', $now + 2 * HOURSECS);
$completed = demo_assignment($course, 'noodle-completed', 'Completed worksheet', 'All three students have submitted this assignment.', $now + 3 * HOURSECS);
demo_assignment($course, 'noodle-future', 'Future project', 'This assignment is due in 48 hours.', $now + 2 * DAYSECS);
demo_assignment($course, 'noodle-expired', 'Expired exercise', 'This assignment was due an hour before seeding.', $now - HOURSECS);
demo_submission($essay, $students['blair'], true);
demo_submission($essay, $students['casey'], false);
foreach ($students as $student) {
    demo_submission($completed, $student, true);
}
$transaction->allow_commit();

umask(0077);
$tokenpath = '/run/noodle-moodle/token';
if (file_put_contents($tokenpath . '.tmp', $token . "\n", LOCK_EX) === false || !rename($tokenpath . '.tmp', $tokenpath)) {
    throw new RuntimeException('Could not write the monitoring token file');
}
echo "Demo course is ready at $CFG->wwwroot/course/view.php?id=$course->id\n";

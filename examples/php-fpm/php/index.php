<?php

$timeout = getenv('EXECUTION_TIMEOUT') !== false ? (int)getenv('EXECUTION_TIMEOUT') : 120;

set_time_limit($timeout);

function dummy()
{
    sleep(1);
}

function work(int $n)
{
    for ($i = 0; $i < $n; $i++) {
    }

    if (time() % 2 === 0) {
        dummy();
    }
}

function fastFunction()
{
    work(20000000);
}

function slowFunction()
{
    work(80000000);
}

for (; ;) {
    fastFunction();
    slowFunction();
}

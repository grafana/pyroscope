const pyroscope = require('./profiler');

const port = process.env['PORT'] || 3000;

const region = process.env['REGION'] || "default";

const express = require('express');
const morgan = require('morgan');

const app = express();
app.use(morgan('dev'));

app.get('/', (req, res) => {
    res.send('Available routes are: /bike, /car, /scooter');
});

const genericSearchHandler = (p) => (req, res) => {
    const time = +(new Date()) + p*1000;
    let i = 0;
    while( +(new Date()) < time ) {
        i = i + Math.random();
    }
    res.send('Vehicle found');
}

// function bikeSearchHandler() {
//     return genericSearchHandler(0.2);
// } 
function carSearchHandler() {
    return  genericSearchHandler(1);
}

function scooterSearchHandler() {
    return genericSearchHandler(0.25);
} 

app.get('/bike',    function bikeSearchHandler(req, res) {
    return genericSearchHandler(0.2)(req, res);
});
app.get('/car',     carSearchHandler());
app.get('/scooter', scooterSearchHandler());

pyroscope.start({lineNumbers: true, name: 'region'});

app.listen(port, () => {
    console.log(`Server has started on port ${port}, use http://localhost:${port}`);
});

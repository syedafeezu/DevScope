const fs = require('fs');

function calculateMetric(data) {
    let sum = 0;
    for (let x of data) {
        sum += x;
    }
    return sum / data.length;
}

exports.processData = function (data) {
    console.log("Analyzing data stream...");
    return calculateMetric(data);
};

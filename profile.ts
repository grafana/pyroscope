/* eslint-disable block-scoped-var */
/* eslint-disable no-plusplus */
/* eslint-disable vars-on-top */
/* eslint-disable no-var */
import './scripts/nodejs-profile';

const normalSieve = (n) => {
  return new Promise((resolve, reject) => {
    // Eratosthenes algorithm to find all primes under n
    const array = [];
    const upperLimit = Math.sqrt(n);
    const output = [];

    const makeArray = () => {
      // Make an array from 2 to (n - 1)
      for (var i = 0; i < n; i++) {
        array.push(true);
      }
    };


    function removeNotPrimes() {
      // Remove multiples of primes starting from 2, 3, 5,...
      for (var i = 2; i <= upperLimit; i++) {
        if (array[i]) {
          for (let j = i * i; j < n; j += i) {
            array[j] = false;
          }
        }
      }
    }

    removeNotPrimes();

    function collectPrimes() {
      // All array[i] set to true are primes
      for (var i = 2; i < n; i++) {
        if (array[i]) {
          output.push(i);
        }
      }
    }
    makeArray();
    removeNotPrimes();
    collectPrimes();

    resolve(output.length);
  });
};

function run() {
  let tries = 0;
  while(true) {
    console.log(`New sieve: ${tries}`);
    normalSieve(32768);
    tries++;
    if (tries > 3) { return; }
    console.log(`Sieve has been built ${tries}`);
  }
}

run();
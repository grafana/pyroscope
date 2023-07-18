const fs = require('fs');
const args = process.argv.slice(2);

if (args.length != 1) {
  console.error('Usage ./obfuscate [filepath]');
  process.exit(1);
}
// TODO(eh-am): read from stdin if available
const filename = args[0];
const data = JSON.parse(fs.readFileSync(filename));

function randomName() {
  let r = (Math.random() + 1).toString(36).substring(7);
  return r;
}

data.metadata.name = randomName();
data.flamebearer.names = data.flamebearer.names.map(randomName);

console.log(JSON.stringify(data));

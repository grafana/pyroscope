// MIT License
//
// Copyright (c) 2020 half-halt
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.
//
// This compoennt is based on https://github.com/half-halt/svg-jest
const path = require('path');

function buildModule(functionName, pathname, filename) {
  return `
const React = require('react');
const ${functionName} = (props) => 
{
  	return React.createElement('svg', { 
  		...props, 
		'data-jest-file-name': '${pathname}',
		'data-jest-svg-name': '${filename}',
		'data-testid': '${filename}'
	});
}
module.exports = ${functionName};
`;
}

function createFunctionName(base) {
  const words = base.split(/\W+/);
  return words.reduce((identifer, word) => {
    return identifer + word.substr(0, 1).toUpperCase() + word.substr(1);
  }, '');
}

function processSvg(contents, filename) {
  const parts = path.parse(filename);
  if (parts.ext.toLowerCase() === '.svg') {
    const functionName = createFunctionName(parts.name);
    return { code: buildModule(functionName, parts.base, parts.name) };
  }

  return { code: contents };
}

module.exports = {
  process: processSvg,
};

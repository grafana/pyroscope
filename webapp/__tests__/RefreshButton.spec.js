import React from 'react';
import renderer from 'react-test-renderer';

import { configure, mount, shallow, render } from 'enzyme';
import Adapter from 'enzyme-adapter-react-16';

import RefreshButton from '../javascript/components/RefreshButton.jsx';

jest.mock('react-redux', () => ({
  useDispatch: () => jest.fn(),
}));

configure({ adapter: new Adapter() });

describe('RefreshButton', () => {
  it('render correctly RefreshButton component', () => {
    const RefreshButtonComponent = renderer.create(<RefreshButton />).toJSON();
    expect(RefreshButtonComponent).toMatchSnapshot();
  });
  it('When refresh button is clicked, Flamegraph component should update', () => {
    const wrapper = shallow(<RefreshButton />);
    expect(wrapper.find('button').length).toEqual(1);
    wrapper.find('button').simulate('click');
  });
});

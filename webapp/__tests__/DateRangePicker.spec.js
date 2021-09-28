import React from 'react';
import renderer from 'react-test-renderer';

import { configure, mount, shallow, render } from 'enzyme';
import Adapter from 'enzyme-adapter-react-16';

import DateRangePicker from '../javascript/components/DateRangePicker.jsx';

jest.mock('react-redux', () => ({
  connect: () => jest.fn(),
  useSelector: () => ({
    from: 'state',
    until: 'state',
  }),
  useDispatch: () => jest.fn(),
}));

configure({ adapter: new Adapter() });

describe('DateRangePicker', () => {
  it('When date changes, flamegraph component should update', () => {
    const wrapper = shallow(<DateRangePicker />);
    expect(wrapper.find('button').length).toEqual(17);

    expect(wrapper.find('button').at(1).text()).toBe('Last 5 minutes');
    expect(wrapper.find('button').at(2).text()).toBe('Last 15 minutes');
    expect(wrapper.find('button').at(3).text()).toBe('Last 30 minutes');
    expect(wrapper.find('button').at(4).text()).toBe('Last 1 hour');

    wrapper.find('button').at(1).simulate('click');
  });
});

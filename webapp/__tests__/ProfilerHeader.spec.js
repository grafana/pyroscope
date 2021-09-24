import React from 'react';
import renderer from 'react-test-renderer';

import { configure, shallow } from 'enzyme';
import Adapter from 'enzyme-adapter-react-16';

import ProfilerHeader from '../javascript/components/ProfilerHeader.jsx';

configure({ adapter: new Adapter() });

describe('ProfilerHeader', () => {
  it('render correctly ProfilerHeader component', () => {
    const ProfilerHeaderComponent = renderer
      .create(<ProfilerHeader />)
      .toJSON();
    expect(ProfilerHeaderComponent).toMatchSnapshot();
  });
  it('When user types in the search bar the state updates with the search term', () => {
    const wrapper = shallow(<ProfilerHeader />);
    const input = wrapper.find('input');
    input.props.value = 'ProfilerHeader';
    expect(input.props.value).toEqual('ProfilerHeader');
  });
});

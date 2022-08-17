import { formatTitle } from './formatTitle';
import { brandQuery } from '@webapp/models/query';

describe('format title', () => {
  describe('when both left and right query are falsy', () => {
    it('returns only the page title', () => {
      expect(formatTitle('mypage')).toBe('mypage');
      expect(formatTitle('mypage', brandQuery(''), brandQuery(''))).toBe(
        'mypage'
      );
    });
  });

  describe('when only a single query is set', () => {
    it('sets it correctly', () => {
      expect(formatTitle('mypage', brandQuery('myquery'))).toBe(
        'mypage | myquery'
      );
    });
  });

  describe('when both queries are set', () => {
    it('sets it correctly', () => {
      expect(
        formatTitle('mypage', brandQuery('myquery1'), brandQuery('myquery2'))
      ).toBe('mypage | myquery1 and myquery2');
    });
  });
});

import React, { useCallback, RefObject, Dispatch, SetStateAction } from 'react';

import type { Units } from '@pyroscope/models/src';
import { Tooltip, TooltipData } from './Tooltip';
import { getFormatter } from '../format/format';

export interface TableTooltipProps {
  tableBodyRef: RefObject<HTMLTableSectionElement>;
  numTicks: number;
  sampleRate: number;
  units: Units;
}

export default function TableTooltip({
  numTicks,
  sampleRate,
  units,
  tableBodyRef,
}: TableTooltipProps) {
  const formatter = getFormatter(numTicks, sampleRate, units);
  const totalFlamebearer = formatter.format(numTicks, sampleRate);

  const setTooltipContent = useCallback(
    (
      setContent: Dispatch<
        SetStateAction<{
          title: {
            text: string;
            diff: {
              text: string;
              color: string;
            };
          };
          tooltipData: TooltipData[];
        }>
      >,
      onMouseOut: () => void,
      e: MouseEvent
    ) => {
      const tableRowElementData = (e.target as Element).closest('tr')?.dataset
        .row;

      if (!tableRowElementData) {
        onMouseOut();
        return;
      }
      const [
        functionName,
        selfValue,
        totalValue,
        // type, for diff view
      ] = tableRowElementData.split(';');

      // think about better way. maybe return value with no units from format method as well ?
      const selfFormatted = formatter.format(
        parseInt(selfValue, 10),
        sampleRate
      );
      const totalFormated = formatter.format(
        parseInt(totalValue, 10),
        sampleRate
      );

      const totalFlamebearerSplitted = totalFlamebearer.split(' ');
      const totalFlamebearerNoUnitsValue =
        totalFlamebearerSplitted[0] === '<'
          ? totalFlamebearerSplitted[1]
          : totalFlamebearerSplitted[0];

      const selfSplitted = selfFormatted.split(' ');
      const selfNoUnitsValue =
        selfSplitted[0] === '<' ? selfSplitted[1] : selfSplitted[0];

      const totalSplitted = totalFormated.split(' ');
      const totalNoUnitsValue =
        totalSplitted[0] === '<' ? totalSplitted[1] : totalSplitted[0];

      const newContent: TooltipData = {
        units,
        self: `${selfFormatted}(${(
          (parseFloat(selfNoUnitsValue) /
            parseFloat(totalFlamebearerNoUnitsValue)) *
          100
        ).toFixed(2)}%)`,
        total: `${totalFormated}(${(
          (parseFloat(totalNoUnitsValue) /
            parseFloat(totalFlamebearerNoUnitsValue)) *
          100
        ).toFixed(2)}%)`,
        tooltipType: 'table',
      };

      setContent({
        title: {
          text: functionName,
          diff: {
            text: '',
            color: '',
          },
        },
        tooltipData: [newContent],
      });
    },
    [tableBodyRef, numTicks, formatter, sampleRate]
  );

  return (
    <Tooltip
      dataSourceRef={tableBodyRef}
      shouldShowTitle={false}
      clickInfoSide="left"
      setTooltipContent={setTooltipContent}
    />
  );
}

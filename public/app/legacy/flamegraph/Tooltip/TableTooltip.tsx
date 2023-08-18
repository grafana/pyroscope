import React, { useCallback, RefObject, Dispatch, SetStateAction } from 'react';
import { Units } from '@pyroscope/legacy/models/units';

import type { FlamegraphPalette } from '../FlameGraph/FlameGraphComponent/colorPalette';
import { Tooltip, TooltipData } from './Tooltip';
import { formatDouble } from './FlamegraphTooltip';
import { getFormatter } from '../format/format';

export interface TableTooltipProps {
  tableBodyRef: RefObject<HTMLTableSectionElement>;
  numTicks: number;
  sampleRate: number;
  units: Units;
  palette: FlamegraphPalette;
}

export default function TableTooltip({
  numTicks,
  sampleRate,
  units,
  tableBodyRef,
  palette,
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
      const [format, functionName, ...rowValues] =
        tableRowElementData.split(';');

      switch (format) {
        case 'single': {
          const [self, total] = rowValues;
          const selfFormatted = formatter.format(
            parseInt(self, 10),
            sampleRate
          );
          const totalFormated = formatter.format(
            parseInt(total, 10),
            sampleRate
          );
          // todo: i think it will be good to decrease number of calculations here
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
          break;
        }
        case 'double': {
          const [totalLeft, leftTicks, totalRight, rightTicks] = rowValues;
          const d = formatDouble(
            {
              formatter,
              sampleRate,
              totalLeft: parseInt(totalLeft, 10),
              leftTicks: parseInt(leftTicks, 10),
              totalRight: parseInt(totalRight, 10),
              rightTicks: parseInt(rightTicks, 10),
              title: functionName,
              units,
            },
            palette
          );

          setContent({
            title: d.title,
            tooltipData: d.tooltipData,
          });

          break;
        }
        default:
          break;
      }
    },
    [formatter, sampleRate, palette, totalFlamebearer, units]
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

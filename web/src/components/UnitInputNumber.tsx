import { Input, InputNumber, Space, type InputNumberProps } from 'antd';

type UnitInputNumberProps = InputNumberProps<number> & {
  unit: string;
};

export function UnitInputNumber({ unit, style, ...props }: UnitInputNumberProps) {
  return (
    <Space.Compact block style={style}>
      <InputNumber<number> {...props} style={{ flex: 1, width: '100%' }} />
      <Input
        aria-hidden
        readOnly
        tabIndex={-1}
        value={unit}
        style={{ width: 42, textAlign: 'center', pointerEvents: 'none' }}
      />
    </Space.Compact>
  );
}

import type { App, Component } from 'vue'
import { defineAsyncComponent } from 'vue'
import ElAlert from 'element-plus/es/components/alert/index'
import ElButton from 'element-plus/es/components/button/index'
import ElForm, { ElFormItem } from 'element-plus/es/components/form/index'
import ElIcon from 'element-plus/es/components/icon/index'
import ElInput from 'element-plus/es/components/input/index'
import ElLoading from 'element-plus/es/components/loading/index'

import 'element-plus/es/components/alert/style/css'
import 'element-plus/es/components/button/style/css'
import 'element-plus/es/components/form/style/css'
import 'element-plus/es/components/form-item/style/css'
import 'element-plus/es/components/icon/style/css'
import 'element-plus/es/components/input/style/css'
import 'element-plus/es/components/loading/style/css'
import 'element-plus/es/components/message/style/css'
import 'element-plus/es/components/message-box/style/css'

const syncComponents = [
  ElAlert,
  ElButton,
  ElForm,
  ElFormItem,
  ElIcon,
  ElInput,
] as const

type AsyncElementPlusLoader = () => Promise<Component>

function loadDefaultComponent(
  moduleLoader: () => Promise<{ default: Component }>,
  styleLoader: () => Promise<unknown>,
): AsyncElementPlusLoader {
  return async () => {
    const [mod] = await Promise.all([
      moduleLoader(),
      styleLoader(),
    ])

    return mod.default
  }
}

function loadNamedComponent(
  moduleLoader: () => Promise<Record<string, unknown>>,
  exportName: string,
  styleLoader: () => Promise<unknown>,
): AsyncElementPlusLoader {
  return async () => {
    const [mod] = await Promise.all([
      moduleLoader(),
      styleLoader(),
    ])

    return mod[exportName] as Component
  }
}

const asyncComponents: Array<{ name: string, loader: AsyncElementPlusLoader }> = [
  {
    name: 'ElCheckbox',
    loader: loadDefaultComponent(
      () => import('element-plus/es/components/checkbox/index'),
      () => import('element-plus/es/components/checkbox/style/css'),
    ),
  },
  {
    name: 'ElCheckboxGroup',
    loader: loadNamedComponent(
      () => import('element-plus/es/components/checkbox/index'),
      'ElCheckboxGroup',
      () => import('element-plus/es/components/checkbox/style/css'),
    ),
  },
  {
    name: 'ElDatePicker',
    loader: loadDefaultComponent(
      () => import('element-plus/es/components/date-picker/index'),
      () => import('element-plus/es/components/date-picker/style/css'),
    ),
  },
  {
    name: 'ElDialog',
    loader: loadDefaultComponent(
      () => import('element-plus/es/components/dialog/index'),
      () => import('element-plus/es/components/dialog/style/css'),
    ),
  },
  {
    name: 'ElDrawer',
    loader: loadDefaultComponent(
      () => import('element-plus/es/components/drawer/index'),
      () => import('element-plus/es/components/drawer/style/css'),
    ),
  },
  {
    name: 'ElDropdown',
    loader: loadDefaultComponent(
      () => import('element-plus/es/components/dropdown/index'),
      () => import('element-plus/es/components/dropdown/style/css'),
    ),
  },
  {
    name: 'ElDropdownItem',
    loader: loadNamedComponent(
      () => import('element-plus/es/components/dropdown/index'),
      'ElDropdownItem',
      () => import('element-plus/es/components/dropdown/style/css'),
    ),
  },
  {
    name: 'ElDropdownMenu',
    loader: loadNamedComponent(
      () => import('element-plus/es/components/dropdown/index'),
      'ElDropdownMenu',
      () => import('element-plus/es/components/dropdown/style/css'),
    ),
  },
  {
    name: 'ElEmpty',
    loader: loadDefaultComponent(
      () => import('element-plus/es/components/empty/index'),
      () => import('element-plus/es/components/empty/style/css'),
    ),
  },
  {
    name: 'ElInputNumber',
    loader: loadDefaultComponent(
      () => import('element-plus/es/components/input-number/index'),
      () => import('element-plus/es/components/input-number/style/css'),
    ),
  },
  {
    name: 'ElOption',
    loader: loadNamedComponent(
      () => import('element-plus/es/components/select/index'),
      'ElOption',
      () => import('element-plus/es/components/select/style/css'),
    ),
  },
  {
    name: 'ElPagination',
    loader: loadDefaultComponent(
      () => import('element-plus/es/components/pagination/index'),
      () => import('element-plus/es/components/pagination/style/css'),
    ),
  },
  {
    name: 'ElProgress',
    loader: loadDefaultComponent(
      () => import('element-plus/es/components/progress/index'),
      () => import('element-plus/es/components/progress/style/css'),
    ),
  },
  {
    name: 'ElRadioButton',
    loader: loadNamedComponent(
      () => import('element-plus/es/components/radio/index'),
      'ElRadioButton',
      () => import('element-plus/es/components/radio/style/css'),
    ),
  },
  {
    name: 'ElRadioGroup',
    loader: loadNamedComponent(
      () => import('element-plus/es/components/radio/index'),
      'ElRadioGroup',
      () => import('element-plus/es/components/radio/style/css'),
    ),
  },
  {
    name: 'ElSelect',
    loader: loadDefaultComponent(
      () => import('element-plus/es/components/select/index'),
      () => import('element-plus/es/components/select/style/css'),
    ),
  },
  {
    name: 'ElSkeleton',
    loader: loadDefaultComponent(
      () => import('element-plus/es/components/skeleton/index'),
      () => import('element-plus/es/components/skeleton/style/css'),
    ),
  },
  {
    name: 'ElSwitch',
    loader: loadDefaultComponent(
      () => import('element-plus/es/components/switch/index'),
      () => import('element-plus/es/components/switch/style/css'),
    ),
  },
  {
    name: 'ElTable',
    loader: loadDefaultComponent(
      () => import('element-plus/es/components/table/index'),
      () => import('element-plus/es/components/table/style/css'),
    ),
  },
  {
    name: 'ElTableColumn',
    loader: loadNamedComponent(
      () => import('element-plus/es/components/table/index'),
      'ElTableColumn',
      () => import('element-plus/es/components/table/style/css'),
    ),
  },
  {
    name: 'ElTag',
    loader: loadDefaultComponent(
      () => import('element-plus/es/components/tag/index'),
      () => import('element-plus/es/components/tag/style/css'),
    ),
  },
  {
    name: 'ElTooltip',
    loader: loadDefaultComponent(
      () => import('element-plus/es/components/tooltip/index'),
      () => import('element-plus/es/components/tooltip/style/css'),
    ),
  },
]

export function registerElementPlus(app: App) {
  for (const component of syncComponents) {
    app.use(component)
  }

  for (const { name, loader } of asyncComponents) {
    app.component(name, defineAsyncComponent({
      loader,
      suspensible: false,
    }))
  }

  app.use(ElLoading)
}

// Element Plus 2.14 exposes component style loaders through package exports
// but omits declarations for the side-effect-only `style/css` modules. Keep
// this narrow shim until upstream publishes matching .d.ts files.
declare module 'element-plus/es/components/*/style/css' {
  const sideEffectModule: Record<string, never>
  export default sideEffectModule
}

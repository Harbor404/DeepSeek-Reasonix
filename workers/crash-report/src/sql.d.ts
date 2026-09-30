declare module "*.sql?raw" {
  const sql: string;
  export default sql;
}

declare module "*.toml?raw" {
  const toml: string;
  export default toml;
}

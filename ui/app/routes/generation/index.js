import Route from '@ember/routing/route';

export default class GenerationIndexRoute extends Route {
  model() {
    return {
      module_name: '',
      op_type: '',
      model_path: '',
      model_md5: '',
      model_count: '',
      prefetch: '',
      model_concurrency: '',
      deploy_ip: '',
      samosa_logic_worker_num: '',
      extra_env: '',
    };
  }

  setupController(controller, model) {
    controller.set('model', model);
  }
}

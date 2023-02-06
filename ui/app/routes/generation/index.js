import Route from '@ember/routing/route';

export default class GenerationIndexRoute extends Route {
  model() {
    return {
      name: '',
      type: '',
      path: '',
      md5: '',
      count: '',
      prefetch: '',
      con: '',
      ip: '',
      sam: '',
      vara: '',
    };
  }

  setupController(controller, model) {
    controller.set('model', model);
  }
}

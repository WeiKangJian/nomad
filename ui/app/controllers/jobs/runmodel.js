import Controller from '@ember/controller';
import { inject as service } from '@ember/service';

export default class RanController extends Controller {
  @service router;
  onSubmit(id, namespace) {
    this.router.transitionTo('jobs.job', `${id}@${namespace || 'default'}`);
  }
}
